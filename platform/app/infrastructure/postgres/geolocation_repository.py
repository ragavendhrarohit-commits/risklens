from __future__ import annotations

from uuid import UUID

from sqlalchemy import select
from sqlalchemy.orm import Session, sessionmaker

from app.domain.geolocation import AgentGeolocation
from app.infrastructure.postgres.models import AgentGeolocationRow
from app.repositories.geolocation_repository import GeolocationRepository


class PostgresGeolocationRepository(GeolocationRepository):
    def __init__(self, session_factory: sessionmaker[Session]) -> None:
        self._session_factory = session_factory

    def get(self, agent_id: UUID) -> AgentGeolocation | None:
        with self._session_factory() as session:
            row = session.get(AgentGeolocationRow, agent_id)
            return None if row is None else _from_row(row)

    def list_all(self) -> list[AgentGeolocation]:
        with self._session_factory() as session:
            rows = session.execute(select(AgentGeolocationRow)).scalars().all()
            return [_from_row(row) for row in rows]

    def upsert(self, row: AgentGeolocation) -> AgentGeolocation:
        with self._session_factory() as session:
            existing = session.get(AgentGeolocationRow, row.agent_id)
            if existing is None:
                session.add(_to_row(row))
            else:
                existing.public_ip = row.public_ip
                existing.private_ip = row.private_ip
                existing.country = row.country
                existing.state = row.state
                existing.city = row.city
                existing.latitude = row.latitude
                existing.longitude = row.longitude
                existing.timezone = row.timezone
                existing.isp = row.isp
                existing.asn = row.asn
                existing.last_updated = row.last_updated
            session.commit()
            stored = session.get(AgentGeolocationRow, row.agent_id)
            assert stored is not None
            return _from_row(stored)


def _from_row(row: AgentGeolocationRow) -> AgentGeolocation:
    return AgentGeolocation(
        agent_id=row.agent_id,
        public_ip=row.public_ip,
        private_ip=row.private_ip,
        country=row.country,
        state=row.state,
        city=row.city,
        latitude=row.latitude,
        longitude=row.longitude,
        timezone=row.timezone,
        isp=row.isp,
        asn=row.asn,
        last_updated=row.last_updated,
    )


def _to_row(row: AgentGeolocation) -> AgentGeolocationRow:
    return AgentGeolocationRow(
        agent_id=row.agent_id,
        public_ip=row.public_ip,
        private_ip=row.private_ip,
        country=row.country,
        state=row.state,
        city=row.city,
        latitude=row.latitude,
        longitude=row.longitude,
        timezone=row.timezone,
        isp=row.isp,
        asn=row.asn,
        last_updated=row.last_updated,
    )
