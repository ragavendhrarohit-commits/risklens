from __future__ import annotations

from uuid import UUID

from sqlalchemy import select
from sqlalchemy.dialects.postgresql import insert as pg_insert
from sqlalchemy.orm import Session, sessionmaker

from app.domain.device import DeviceInventory
from app.infrastructure.postgres.models import DeviceInventoryRow
from app.repositories.device_inventory_repository import DeviceInventoryRepository


class PostgresDeviceInventoryRepository(DeviceInventoryRepository):
    """PostgreSQL adapter that upserts the latest inventory per AgentID."""

    def __init__(self, session_factory: sessionmaker[Session]) -> None:
        self._session_factory = session_factory

    def upsert(self, inventory: DeviceInventory) -> tuple[DeviceInventory, bool]:
        row = DeviceInventoryRow.from_domain(inventory)
        values = {
            column.name: getattr(row, column.name)
            for column in DeviceInventoryRow.__table__.columns
        }

        with self._session_factory() as session:
            existing = session.get(DeviceInventoryRow, inventory.agent_id)
            created = existing is None

            dialect = session.bind.dialect.name if session.bind is not None else ""
            if dialect == "postgresql":
                stmt = pg_insert(DeviceInventoryRow).values(**values)
                update_cols = {
                    key: stmt.excluded[key]
                    for key in values
                    if key != "agent_id"
                }
                stmt = stmt.on_conflict_do_update(
                    index_elements=[DeviceInventoryRow.agent_id],
                    set_=update_cols,
                )
                session.execute(stmt)
            elif created:
                session.add(row)
            else:
                for key, value in values.items():
                    if key != "agent_id":
                        setattr(existing, key, value)
            session.commit()
            return inventory, created

    def get(self, agent_id: UUID) -> DeviceInventory | None:
        with self._session_factory() as session:
            row = session.get(DeviceInventoryRow, agent_id)
            return row.to_domain() if row is not None else None

    def list(self) -> list[DeviceInventory]:
        with self._session_factory() as session:
            stmt = select(DeviceInventoryRow).order_by(
                DeviceInventoryRow.received_at,
                DeviceInventoryRow.agent_id,
            )
            return [row.to_domain() for row in session.execute(stmt).scalars()]
