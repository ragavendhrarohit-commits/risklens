from __future__ import annotations

from datetime import datetime
from uuid import UUID

from sqlalchemy import select
from sqlalchemy.dialects.postgresql import insert as pg_insert
from sqlalchemy.orm import Session, sessionmaker

from app.domain.fleet import (
    DEFAULT_WINDOWS_POLICY_ID,
    AgentStatusSnapshot,
    Policy,
    PolicyAssignment,
)
from app.infrastructure.postgres.models import AgentPolicyRow, AgentStatusRow, PolicyRow
from app.repositories.fleet_repository import AgentStatusRepository, PolicyRepository


class PostgresAgentStatusRepository(AgentStatusRepository):
    def __init__(self, session_factory: sessionmaker[Session]) -> None:
        self._session_factory = session_factory

    def ensure(self, agent_id: UUID) -> None:
        with self._session_factory() as session:
            if session.get(AgentStatusRow, agent_id) is not None:
                return
            dialect = session.bind.dialect.name if session.bind is not None else ""
            if dialect == "postgresql":
                stmt = (
                    pg_insert(AgentStatusRow)
                    .values(agent_id=agent_id)
                    .on_conflict_do_nothing(index_elements=[AgentStatusRow.agent_id])
                )
                session.execute(stmt)
            else:
                session.add(AgentStatusRow(agent_id=agent_id))
            session.commit()

    def record_seen(self, agent_id: UUID, last_seen: datetime, received_at: datetime) -> None:
        with self._session_factory() as session:
            row = session.get(AgentStatusRow, agent_id)
            if row is None:
                row = AgentStatusRow(agent_id=agent_id)
                session.add(row)
            row.last_seen = last_seen
            row.last_communication_at = _later(row.last_communication_at, received_at)
            session.commit()

    def record_inventory(self, agent_id: UUID, received_at: datetime) -> None:
        with self._session_factory() as session:
            row = session.get(AgentStatusRow, agent_id)
            if row is None:
                row = AgentStatusRow(agent_id=agent_id)
                session.add(row)
            row.last_inventory_at = received_at
            row.last_communication_at = _later(row.last_communication_at, received_at)
            session.commit()

    def get(self, agent_id: UUID) -> AgentStatusSnapshot | None:
        with self._session_factory() as session:
            row = session.get(AgentStatusRow, agent_id)
            return None if row is None else _status_from_row(row)

    def list_map(self) -> dict[UUID, AgentStatusSnapshot]:
        with self._session_factory() as session:
            rows = session.execute(select(AgentStatusRow)).scalars()
            return {row.agent_id: _status_from_row(row) for row in rows}


class PostgresPolicyRepository(PolicyRepository):
    def __init__(self, session_factory: sessionmaker[Session]) -> None:
        self._session_factory = session_factory

    def get_default_policy_id(self) -> UUID:
        return DEFAULT_WINDOWS_POLICY_ID

    def get_policy(self, policy_id: UUID) -> Policy | None:
        with self._session_factory() as session:
            row = session.get(PolicyRow, policy_id)
            return None if row is None else _policy_from_row(row)

    def get_default_policy(self) -> Policy | None:
        return self.get_policy(DEFAULT_WINDOWS_POLICY_ID)

    def assign_default(self, agent_id: UUID, assigned_at: datetime) -> None:
        with self._session_factory() as session:
            if session.get(AgentPolicyRow, agent_id) is not None:
                return
            dialect = session.bind.dialect.name if session.bind is not None else ""
            values = {
                "agent_id": agent_id,
                "policy_id": DEFAULT_WINDOWS_POLICY_ID,
                "assigned_at": assigned_at,
            }
            if dialect == "postgresql":
                stmt = (
                    pg_insert(AgentPolicyRow)
                    .values(**values)
                    .on_conflict_do_nothing(index_elements=[AgentPolicyRow.agent_id])
                )
                session.execute(stmt)
            else:
                session.add(AgentPolicyRow(**values))
            session.commit()

    def get_assignment(self, agent_id: UUID) -> PolicyAssignment | None:
        with self._session_factory() as session:
            stmt = (
                select(AgentPolicyRow, PolicyRow)
                .join(PolicyRow, PolicyRow.id == AgentPolicyRow.policy_id)
                .where(AgentPolicyRow.agent_id == agent_id)
            )
            row = session.execute(stmt).first()
            if row is None:
                return None
            assignment, policy = row
            return PolicyAssignment(
                agent_id=assignment.agent_id,
                policy_id=assignment.policy_id,
                policy_name=policy.name,
                assigned_at=assignment.assigned_at,
                policy=_policy_from_row(policy),
            )

    def list_assignments(self) -> dict[UUID, PolicyAssignment]:
        with self._session_factory() as session:
            stmt = select(AgentPolicyRow, PolicyRow).join(
                PolicyRow, PolicyRow.id == AgentPolicyRow.policy_id
            )
            result: dict[UUID, PolicyAssignment] = {}
            for assignment, policy in session.execute(stmt).all():
                result[assignment.agent_id] = PolicyAssignment(
                    agent_id=assignment.agent_id,
                    policy_id=assignment.policy_id,
                    policy_name=policy.name,
                    assigned_at=assignment.assigned_at,
                    policy=_policy_from_row(policy),
                )
            return result


def _later(current: datetime | None, candidate: datetime) -> datetime:
    if current is None or candidate >= current:
        return candidate
    return current


def _status_from_row(row: AgentStatusRow) -> AgentStatusSnapshot:
    return AgentStatusSnapshot(
        agent_id=row.agent_id,
        last_seen=row.last_seen,
        last_inventory_at=row.last_inventory_at,
        last_communication_at=row.last_communication_at,
        ip_address=row.ip_address,
    )


def _policy_from_row(row: PolicyRow) -> Policy:
    return Policy(
        id=row.id,
        name=row.name,
        heartbeat_interval_seconds=row.heartbeat_interval_seconds,
        collect_devices=row.collect_devices,
        collect_users=row.collect_users,
        collect_software=row.collect_software,
        collect_processes=row.collect_processes,
        collect_services=row.collect_services,
        collect_event_logs=row.collect_event_logs,
        created_at=row.created_at,
    )
