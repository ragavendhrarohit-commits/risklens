from __future__ import annotations

from datetime import datetime, timezone
from uuid import UUID

from app.domain.agent import Agent
from app.domain.device import DeviceInventory
from app.domain.fleet import (
    DEFAULT_WINDOWS_POLICY_NAME,
    NOT_IMPLEMENTED,
    FleetAgentNotFoundError,
    FleetHealth,
    FleetOnlineStatus,
    FleetPage,
    FleetRecord,
    FleetSummary,
    ListFleetQuery,
    Policy,
    compute_fleet_status,
    parse_sort,
)
from app.domain.applications import (
    MAX_LIMIT as APPLICATION_MAX_LIMIT,
    ApplicationActivityPage,
    ListApplicationActivityQuery,
)
from app.domain.website_usage import WebsiteUsageRow, summarize_website_usage
from app.domain.sessions import ListSessionsQuery, SessionPage
from app.domain.software import ListSoftwareQuery, SoftwarePage
from app.domain.user import ListUsersQuery, UserPage
from app.logging.setup import get_logger
from app.repositories.agent_repository import AgentRepository
from app.repositories.device_inventory_repository import DeviceInventoryRepository
from app.repositories.fleet_repository import (
    AgentStatusRepository,
    AgentTagRepository,
    PolicyRepository,
)
from app.repositories.heartbeat_repository import HeartbeatRepository
from app.services.application_activity_service import ApplicationActivityService
from app.services.session_service import SessionService
from app.services.software_service import SoftwareInventoryService
from app.services.user_service import UserInventoryService

_logger = get_logger("risklens.fleet")


class FleetProjection:
    """Writes agent_status and default policy after Sprint 12 mutations."""

    def __init__(
        self,
        status: AgentStatusRepository,
        policies: PolicyRepository,
        *,
        clock=None,
    ) -> None:
        self._status = status
        self._policies = policies
        self._clock = clock or (lambda: datetime.now(timezone.utc))

    def on_registered(self, agent_id: UUID) -> None:
        now = self._clock()
        self._status.ensure(agent_id)
        self._policies.assign_default(agent_id, now)

    def on_heartbeat(self, agent_id: UUID, last_seen: datetime, received_at: datetime) -> None:
        self._status.record_seen(agent_id, last_seen, received_at)

    def on_inventory(self, agent_id: UUID, received_at: datetime) -> None:
        self._status.record_inventory(agent_id, received_at)


class FleetService:
    """Read orchestrator for the Fleet console. Reuses Sprint 12 inventory."""

    def __init__(
        self,
        agents: AgentRepository,
        inventory: DeviceInventoryRepository,
        heartbeats: HeartbeatRepository,
        status: AgentStatusRepository,
        policies: PolicyRepository,
        tags: AgentTagRepository,
        users: UserInventoryService,
        software: SoftwareInventoryService,
        sessions: SessionService,
        applications: ApplicationActivityService,
        *,
        clock=None,
    ) -> None:
        self._agents = agents
        self._inventory = inventory
        self._heartbeats = heartbeats
        self._status = status
        self._policies = policies
        self._tags = tags
        self._users = users
        self._software = software
        self._sessions = sessions
        self._applications = applications
        self._clock = clock or (lambda: datetime.now(timezone.utc))

    def list(self, query: ListFleetQuery) -> FleetPage:
        records = self._all_records()
        summary = self._summary(records)
        filtered = [row for row in records if self._matches(row, query)]
        field, descending = parse_sort(query.sort)
        filtered.sort(key=lambda row: _sort_key(row, field), reverse=descending)
        total = len(filtered)
        page = filtered[query.offset : query.offset + query.limit]
        return FleetPage(
            items=page,
            summary=summary,
            total=total,
            limit=query.limit,
            offset=query.offset,
        )

    def get(self, agent_id: UUID) -> FleetRecord:
        if not self._agents.exists(agent_id):
            raise FleetAgentNotFoundError(agent_id)
        records = {row.agent_id: row for row in self._all_records()}
        record = records.get(agent_id)
        if record is None:
            raise FleetAgentNotFoundError(agent_id)
        return record

    def users(self, agent_id: UUID, *, limit: int, offset: int, sort: str) -> UserPage:
        self._require_agent(agent_id)
        return self._users.list(
            ListUsersQuery(agent_id=agent_id, limit=limit, offset=offset, sort=sort)
        )

    def software(self, agent_id: UUID, *, limit: int, offset: int, sort: str) -> SoftwarePage:
        self._require_agent(agent_id)
        return self._software.list(
            ListSoftwareQuery(agent_id=agent_id, limit=limit, offset=offset, sort=sort)
        )

    def sessions(self, agent_id: UUID, *, limit: int, offset: int, sort: str) -> SessionPage:
        self._require_agent(agent_id)
        record = self.get(agent_id)
        if record.online_status == FleetOnlineStatus.offline.value:
            self._sessions.close_open_for_agent(
                agent_id, record.last_seen or self._clock()
            )
        return self._sessions.list(
            ListSessionsQuery(agent_id=agent_id, limit=limit, offset=offset, sort=sort)
        )

    def applications(
        self, agent_id: UUID, *, limit: int, offset: int, sort: str
    ) -> ApplicationActivityPage:
        self._require_agent(agent_id)
        return self._applications.list(
            ListApplicationActivityQuery(agent_id=agent_id, limit=limit, offset=offset, sort=sort)
        )

    def website_usage(self, agent_id: UUID) -> list[WebsiteUsageRow]:
        """Summarize website time from existing Application Activity window titles."""
        self._require_agent(agent_id)
        records = []
        offset = 0
        while True:
            page = self._applications.list(
                ListApplicationActivityQuery(
                    agent_id=agent_id,
                    limit=APPLICATION_MAX_LIMIT,
                    offset=offset,
                    sort="-started_at",
                )
            )
            records.extend(page.items)
            offset += len(page.items)
            if not page.items or offset >= page.total:
                break
        return summarize_website_usage(records)

    def health(self, agent_id: UUID) -> FleetHealth:
        record = self.get(agent_id)
        return FleetHealth(
            status=record.online_status,
            heartbeat_age_seconds=record.heartbeat_age_seconds,
            last_inventory_at=record.last_inventory_at,
            last_communication_at=record.last_communication_at,
            cpu_usage=NOT_IMPLEMENTED,
            memory_usage=NOT_IMPLEMENTED,
            disk_usage=NOT_IMPLEMENTED,
            memory_bytes=record.memory_bytes,
            disk_total_bytes=record.disk_total_bytes,
            disk_free_bytes=record.disk_free_bytes,
        )

    def policy(self, agent_id: UUID) -> Policy:
        self._require_agent(agent_id)
        assignment = self._policies.get_assignment(agent_id)
        if assignment is not None:
            return assignment.policy
        default = self._policies.get_default_policy()
        if default is None:
            raise FleetAgentNotFoundError(agent_id)
        return default

    def require_agent(self, agent_id: UUID) -> None:
        self._require_agent(agent_id)

    def _require_agent(self, agent_id: UUID) -> Agent:
        agent = self._agents.get(agent_id)
        if agent is None:
            raise FleetAgentNotFoundError(agent_id)
        return agent

    def _all_records(self) -> list[FleetRecord]:
        agents = self._agents.list()
        inventories = {row.agent_id: row for row in self._inventory.list()}
        heartbeats = self._heartbeats.get_latest_map([agent.agent_id for agent in agents])
        status_map = self._status.list_map()
        assignments = self._policies.list_assignments()
        tags_map = self._tags.list_map([agent.agent_id for agent in agents])
        now = self._clock()
        return [
            self._to_record(
                agent,
                inventories.get(agent.agent_id),
                status_map.get(agent.agent_id),
                heartbeats.get(agent.agent_id).heartbeat_time if agent.agent_id in heartbeats else None,
                heartbeats.get(agent.agent_id).received_at if agent.agent_id in heartbeats else None,
                assignments.get(agent.agent_id).policy_name
                if agent.agent_id in assignments
                else DEFAULT_WINDOWS_POLICY_NAME,
                assignments.get(agent.agent_id).policy_id if agent.agent_id in assignments else None,
                tags_map.get(agent.agent_id, []),
                now,
            )
            for agent in agents
        ]

    def _to_record(
        self,
        agent: Agent,
        inventory: DeviceInventory | None,
        status,
        heartbeat_time: datetime | None,
        heartbeat_received: datetime | None,
        policy_name: str,
        policy_id: UUID | None,
        tags: list[str],
        now: datetime,
    ) -> FleetRecord:
        last_seen = None
        if status is not None and status.last_seen is not None:
            last_seen = status.last_seen
        elif heartbeat_time is not None:
            last_seen = heartbeat_time

        last_inventory_at = None
        if status is not None and status.last_inventory_at is not None:
            last_inventory_at = status.last_inventory_at
        elif inventory is not None:
            last_inventory_at = inventory.received_at

        last_communication_at = None
        if status is not None:
            last_communication_at = status.last_communication_at
        candidates = [item for item in (heartbeat_received, last_inventory_at) if item is not None]
        if last_communication_at is None and candidates:
            last_communication_at = max(candidates)

        online_status, age, healthy = compute_fleet_status(
            last_seen, has_inventory=inventory is not None, now=now
        )
        if online_status is FleetOnlineStatus.offline:
            self._sessions.close_open_for_agent(agent.agent_id, last_seen or now)
        ip_address = status.ip_address if status is not None else None
        return FleetRecord(
            agent_id=agent.agent_id,
            hostname=(inventory.hostname if inventory is not None else ""),
            machine_name=(
                inventory.machine_name if inventory is not None else agent.machine_name
            ),
            agent_version=(
                inventory.agent_version if inventory is not None else agent.agent_version
            ),
            operating_system=(
                inventory.operating_system if inventory is not None else agent.operating_system
            ),
            current_user=(inventory.current_user if inventory is not None else ""),
            registration_status=agent.status.value,
            online_status=online_status.value,
            last_seen=last_seen,
            last_inventory_at=last_inventory_at,
            ip_address=ip_address,
            tags=list(tags),
            policy_name=policy_name,
            policy_id=policy_id,
            heartbeat_age_seconds=age,
            last_communication_at=last_communication_at,
            healthy=healthy,
            os_version=(inventory.os_version if inventory is not None else ""),
            architecture=(inventory.architecture if inventory is not None else ""),
            manufacturer=(inventory.manufacturer if inventory is not None else ""),
            model=(inventory.model if inventory is not None else ""),
            windows_build=(inventory.windows_build if inventory is not None else ""),
            cpu_model=(inventory.cpu_model if inventory is not None else ""),
            cpu_cores=(inventory.cpu_cores if inventory is not None else None),
            memory_bytes=(inventory.memory_bytes if inventory is not None else None),
            disk_total_bytes=(inventory.disk_total_bytes if inventory is not None else None),
            disk_free_bytes=(inventory.disk_free_bytes if inventory is not None else None),
            serial_number=(inventory.serial_number if inventory is not None else ""),
        )

    def _summary(self, records: list[FleetRecord]) -> FleetSummary:
        counts = {
            FleetOnlineStatus.online.value: 0,
            FleetOnlineStatus.offline.value: 0,
            FleetOnlineStatus.warning.value: 0,
            FleetOnlineStatus.never_seen.value: 0,
        }
        healthy = 0
        for row in records:
            counts[row.online_status] = counts.get(row.online_status, 0) + 1
            if row.healthy:
                healthy += 1
        return FleetSummary(
            total_agents=len(records),
            online=counts[FleetOnlineStatus.online.value],
            offline=counts[FleetOnlineStatus.offline.value],
            warning=counts[FleetOnlineStatus.warning.value],
            never_seen=counts[FleetOnlineStatus.never_seen.value],
            healthy=healthy,
        )

    @staticmethod
    def _matches(row: FleetRecord, query: ListFleetQuery) -> bool:
        if query.q:
            needle = query.q.casefold()
            haystack = " ".join(
                [
                    str(row.agent_id),
                    row.hostname,
                    row.machine_name,
                    row.operating_system,
                    row.current_user,
                ]
            ).casefold()
            if needle not in haystack:
                return False
        if query.status == "healthy":
            if not row.healthy:
                return False
        elif query.status and row.online_status != query.status:
            return False
        if query.tag and query.tag not in row.tags:
            return False
        if query.policy_id is not None and row.policy_id != query.policy_id:
            return False
        return True


def _sort_key(row: FleetRecord, field: str):
    value = getattr(row, field)
    if isinstance(value, str):
        return value.casefold()
    if value is None:
        return ""
    return value
