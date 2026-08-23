from __future__ import annotations

from datetime import datetime, timezone
from uuid import UUID

from app.domain.fleet import FleetOnlineStatus, compute_fleet_status
from app.domain.geolocation import (
    AgentGeolocation,
    GeolocationDevice,
    GeolocationPage,
    GeolocationSummary,
    GeoLookupProvider,
    ListGeolocationQuery,
    UpsertGeolocationCommand,
    is_public_ip,
)
from app.domain.heartbeat import RecordHeartbeatCommand
from app.logging.setup import get_logger
from app.repositories.agent_repository import AgentRepository
from app.repositories.device_inventory_repository import DeviceInventoryRepository
from app.repositories.fleet_repository import AgentStatusRepository
from app.repositories.geolocation_repository import GeolocationRepository

_logger = get_logger("risklens.geolocation")


class GeolocationNotFoundError(LookupError):
    def __init__(self, agent_id: UUID) -> None:
        super().__init__(f"Geolocation not found for agent {agent_id}")
        self.agent_id = agent_id


class GeolocationService:
    """Stores IP-based endpoint locations and serves the Geo Location console."""

    def __init__(
        self,
        agents: AgentRepository,
        inventory: DeviceInventoryRepository,
        status: AgentStatusRepository,
        geolocation: GeolocationRepository,
        lookup: GeoLookupProvider,
        *,
        clock=None,
    ) -> None:
        self._agents = agents
        self._inventory = inventory
        self._status = status
        self._geolocation = geolocation
        self._lookup = lookup
        self._clock = clock or (lambda: datetime.now(timezone.utc))

    def ingest_heartbeat(
        self,
        command: RecordHeartbeatCommand,
        *,
        client_ip: str | None = None,
    ) -> None:
        public_ip = _normalize_ip(command.public_ip)
        if not is_public_ip(public_ip):
            public_ip = _normalize_ip(client_ip) if is_public_ip(client_ip) else None
        private_ip = _normalize_ip(command.private_ip)
        if public_ip is None and private_ip is None and not (command.hostname or "").strip():
            return

        now = self._clock()
        existing = self._geolocation.get(command.agent_id)
        stored_public = existing.public_ip if existing is not None else None
        should_lookup = public_ip is not None and public_ip != stored_public

        geo_fields = {
            "country": existing.country if existing is not None else None,
            "state": existing.state if existing is not None else None,
            "city": existing.city if existing is not None else None,
            "latitude": existing.latitude if existing is not None else None,
            "longitude": existing.longitude if existing is not None else None,
            "timezone": existing.timezone if existing is not None else None,
            "isp": existing.isp if existing is not None else None,
            "asn": existing.asn if existing is not None else None,
        }
        if should_lookup and public_ip is not None:
            result = self._lookup.lookup(public_ip)
            if result is not None:
                geo_fields = {
                    "country": result.country,
                    "state": result.state,
                    "city": result.city,
                    "latitude": result.latitude,
                    "longitude": result.longitude,
                    "timezone": result.timezone,
                    "isp": result.isp,
                    "asn": result.asn,
                }
                _logger.info(
                    "geolocation refreshed",
                    extra={"agent_id": str(command.agent_id), "public_ip": public_ip},
                )

        resolved_public = public_ip if public_ip is not None else stored_public
        resolved_private = private_ip if private_ip is not None else (
            existing.private_ip if existing is not None else None
        )
        row = AgentGeolocation(
            agent_id=command.agent_id,
            public_ip=resolved_public,
            private_ip=resolved_private,
            last_updated=now if should_lookup or existing is None else existing.last_updated,
            **geo_fields,
        )
        self._geolocation.upsert(row)

    def upsert(self, command: UpsertGeolocationCommand) -> AgentGeolocation:
        self.ingest_heartbeat(
            RecordHeartbeatCommand(
                agent_id=command.agent_id,
                install_id="",
                timestamp=command.at,
                public_ip=command.public_ip,
                private_ip=command.private_ip,
                hostname=command.hostname,
            )
        )
        stored = self._geolocation.get(command.agent_id)
        assert stored is not None
        return stored

    def list(self, query: ListGeolocationQuery) -> GeolocationPage:
        devices = self._all_devices()
        filtered = self._apply_filters(devices, query)
        summary = self._summary(devices)
        return GeolocationPage(summary=summary, items=filtered, total=len(filtered))

    def get(self, agent_id: UUID) -> GeolocationDevice:
        for device in self._all_devices():
            if device.agent_id == agent_id:
                return device
        raise GeolocationNotFoundError(agent_id)

    def _all_devices(self) -> list[GeolocationDevice]:
        now = self._clock()
        agents = {agent.agent_id: agent for agent in self._agents.list()}
        inventory_map = {row.agent_id: row for row in self._inventory.list()}
        status_map = self._status.list_map()
        geo_map = {row.agent_id: row for row in self._geolocation.list_all()}

        devices: list[GeolocationDevice] = []
        for agent_id, agent in agents.items():
            geo = geo_map.get(agent_id)
            if geo is None or not self._has_location(geo):
                continue

            inventory = inventory_map.get(agent_id)
            status = status_map.get(agent_id)
            last_seen = status.last_seen if status is not None else None
            online_status, _, _ = compute_fleet_status(
                last_seen,
                has_inventory=inventory is not None,
                now=now,
            )
            computer = (
                inventory.hostname
                if inventory is not None and inventory.hostname
                else agent.machine_name
            )
            hostname = inventory.hostname if inventory is not None and inventory.hostname else computer
            devices.append(
                GeolocationDevice(
                    agent_id=agent_id,
                    computer=computer,
                    user=(inventory.current_user if inventory is not None else ""),
                    hostname=hostname,
                    status=online_status.value,
                    city=geo.city,
                    state=geo.state,
                    country=geo.country,
                    public_ip=geo.public_ip,
                    private_ip=geo.private_ip,
                    isp=geo.isp,
                    timezone=geo.timezone,
                    latitude=geo.latitude,
                    longitude=geo.longitude,
                    asn=geo.asn,
                    last_seen=last_seen,
                    operating_system=(
                        inventory.operating_system
                        if inventory is not None
                        else agent.operating_system
                    ),
                    online=online_status is FleetOnlineStatus.online,
                )
            )
        devices.sort(key=lambda item: (item.computer.lower(), str(item.agent_id)))
        return devices

    @staticmethod
    def _has_location(geo: AgentGeolocation) -> bool:
        return bool(
            geo.public_ip
            or geo.country
            or geo.city
            or (geo.latitude is not None and geo.longitude is not None)
        )

    def _summary(self, devices: list[GeolocationDevice]) -> GeolocationSummary:
        with_location = [item for item in devices if item.public_ip or item.country or item.city]
        countries = {item.country for item in with_location if item.country}
        cities = {
            (item.country or "", item.city or "")
            for item in with_location
            if item.city
        }
        return GeolocationSummary(
            managed_devices=len(with_location),
            online_devices=sum(1 for item in with_location if item.online),
            countries=len(countries),
            cities=len(cities),
        )

    def _apply_filters(
        self,
        devices: list[GeolocationDevice],
        query: ListGeolocationQuery,
    ) -> list[GeolocationDevice]:
        needle = query.q.strip().lower()
        status = query.status.strip().lower()
        country = query.country.strip().lower()
        city = query.city.strip().lower()
        isp = query.isp.strip().lower()

        filtered: list[GeolocationDevice] = []
        for item in devices:
            if status == "online" and not item.online:
                continue
            if status == "offline" and item.online:
                continue
            if country and (item.country or "").lower() != country:
                continue
            if city and (item.city or "").lower() != city:
                continue
            if isp and (item.isp or "").lower() != isp:
                continue
            if needle:
                haystack = " ".join(
                    [
                        item.computer,
                        item.user,
                        item.country or "",
                        item.city or "",
                        item.public_ip or "",
                    ]
                ).lower()
                if needle not in haystack:
                    continue
            filtered.append(item)
        return filtered


def _normalize_ip(value: str | None) -> str | None:
    if value is None:
        return None
    cleaned = value.strip()
    return cleaned or None
