from __future__ import annotations

from datetime import datetime, timezone
from threading import RLock
from uuid import UUID

from app.domain.fleet import (
    DEFAULT_WINDOWS_POLICY_ID,
    DEFAULT_WINDOWS_POLICY_NAME,
    AgentStatusSnapshot,
    Policy,
    PolicyAssignment,
)
from app.repositories.fleet_repository import (
    AgentStatusRepository,
    AgentTagRepository,
    PolicyRepository,
)


def _default_policy() -> Policy:
    return Policy(
        id=DEFAULT_WINDOWS_POLICY_ID,
        name=DEFAULT_WINDOWS_POLICY_NAME,
        heartbeat_interval_seconds=30,
        collect_devices=True,
        collect_users=True,
        collect_software=True,
        collect_processes=False,
        collect_services=False,
        collect_event_logs=False,
        created_at=datetime(2026, 8, 15, tzinfo=timezone.utc),
    )


class MemoryAgentStatusRepository(AgentStatusRepository):
    def __init__(self) -> None:
        self._rows: dict[UUID, AgentStatusSnapshot] = {}
        self._lock = RLock()

    def ensure(self, agent_id: UUID) -> None:
        with self._lock:
            self._rows.setdefault(agent_id, AgentStatusSnapshot(agent_id=agent_id))

    def record_seen(self, agent_id: UUID, last_seen: datetime, received_at: datetime) -> None:
        with self._lock:
            current = self._rows.get(agent_id, AgentStatusSnapshot(agent_id=agent_id))
            comm = current.last_communication_at
            if comm is None or received_at >= comm:
                comm = received_at
            self._rows[agent_id] = AgentStatusSnapshot(
                agent_id=agent_id,
                last_seen=last_seen,
                last_inventory_at=current.last_inventory_at,
                last_communication_at=comm,
                ip_address=current.ip_address,
            )

    def record_inventory(self, agent_id: UUID, received_at: datetime) -> None:
        with self._lock:
            current = self._rows.get(agent_id, AgentStatusSnapshot(agent_id=agent_id))
            comm = current.last_communication_at
            if comm is None or received_at >= comm:
                comm = received_at
            self._rows[agent_id] = AgentStatusSnapshot(
                agent_id=agent_id,
                last_seen=current.last_seen,
                last_inventory_at=received_at,
                last_communication_at=comm,
                ip_address=current.ip_address,
            )

    def get(self, agent_id: UUID) -> AgentStatusSnapshot | None:
        with self._lock:
            return self._rows.get(agent_id)

    def list_map(self) -> dict[UUID, AgentStatusSnapshot]:
        with self._lock:
            return dict(self._rows)


class MemoryPolicyRepository(PolicyRepository):
    def __init__(self) -> None:
        self._policies = {DEFAULT_WINDOWS_POLICY_ID: _default_policy()}
        self._assignments: dict[UUID, PolicyAssignment] = {}
        self._lock = RLock()

    def get_default_policy_id(self) -> UUID:
        return DEFAULT_WINDOWS_POLICY_ID

    def get_policy(self, policy_id: UUID) -> Policy | None:
        with self._lock:
            return self._policies.get(policy_id)

    def get_default_policy(self) -> Policy | None:
        return self.get_policy(DEFAULT_WINDOWS_POLICY_ID)

    def assign_default(self, agent_id: UUID, assigned_at: datetime) -> None:
        with self._lock:
            if agent_id in self._assignments:
                return
            policy = self._policies[DEFAULT_WINDOWS_POLICY_ID]
            self._assignments[agent_id] = PolicyAssignment(
                agent_id=agent_id,
                policy_id=policy.id,
                policy_name=policy.name,
                assigned_at=assigned_at,
                policy=policy,
            )

    def get_assignment(self, agent_id: UUID) -> PolicyAssignment | None:
        with self._lock:
            return self._assignments.get(agent_id)

    def list_assignments(self) -> dict[UUID, PolicyAssignment]:
        with self._lock:
            return dict(self._assignments)


class MemoryAgentTagRepository(AgentTagRepository):
    def __init__(self) -> None:
        self._tags: dict[UUID, list[str]] = {}
        self._lock = RLock()

    def list_for_agent(self, agent_id: UUID) -> list[str]:
        with self._lock:
            return list(self._tags.get(agent_id, []))

    def list_map(self, agent_ids: list[UUID]) -> dict[UUID, list[str]]:
        with self._lock:
            return {agent_id: list(self._tags.get(agent_id, [])) for agent_id in agent_ids}
