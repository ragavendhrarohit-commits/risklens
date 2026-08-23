from __future__ import annotations

import ipaddress
from dataclasses import dataclass
from typing import Protocol

import httpx

from app.logging.setup import get_logger

_logger = get_logger("risklens.geolocation.lookup")


@dataclass(frozen=True, slots=True)
class GeoLookupResult:
    country: str | None
    state: str | None
    city: str | None
    latitude: float | None
    longitude: float | None
    timezone: str | None
    isp: str | None
    asn: str | None


class GeoLookupProvider(Protocol):
    def lookup(self, public_ip: str) -> GeoLookupResult | None: ...


def is_public_ip(value: str | None) -> bool:
    if not value or not value.strip():
        return False
    try:
        addr = ipaddress.ip_address(value.strip())
    except ValueError:
        return False
    return not (
        addr.is_private
        or addr.is_loopback
        or addr.is_link_local
        or addr.is_multicast
        or addr.is_reserved
        or addr.is_unspecified
    )


class StubGeoLookupProvider:
    """Deterministic lookup for tests and local development."""

    _DATA: dict[str, GeoLookupResult] = {
        "49.177.1.1": GeoLookupResult(
            country="Australia",
            state="Victoria",
            city="Melbourne",
            latitude=-37.8136,
            longitude=144.9631,
            timezone="Australia/Melbourne",
            isp="Aussie Broadband",
            asn="AS4764 Aussie Broadband Pty Ltd",
        ),
        "1.2.3.4": GeoLookupResult(
            country="Australia",
            state="New South Wales",
            city="Sydney",
            latitude=-33.8688,
            longitude=151.2093,
            timezone="Australia/Sydney",
            isp="Telstra",
            asn="AS1221 Telstra Pty Ltd",
        ),
    }

    def lookup(self, public_ip: str) -> GeoLookupResult | None:
        if not is_public_ip(public_ip):
            return None
        return self._DATA.get(
            public_ip.strip(),
            GeoLookupResult(
                country="Unknown",
                state=None,
                city="Unknown",
                latitude=0.0,
                longitude=0.0,
                timezone="UTC",
                isp="Unknown ISP",
                asn=None,
            ),
        )


class IpApiGeoLookupProvider:
    """Resolve approximate location from a public IPv4/IPv6 address."""

    def __init__(self, *, timeout: float = 3.0) -> None:
        self._timeout = timeout

    def lookup(self, public_ip: str) -> GeoLookupResult | None:
        if not is_public_ip(public_ip):
            return None
        url = (
            f"http://ip-api.com/json/{public_ip.strip()}"
            "?fields=status,country,regionName,city,lat,lon,timezone,isp,as,query"
        )
        try:
            with httpx.Client(timeout=self._timeout) as client:
                response = client.get(url)
                response.raise_for_status()
                payload = response.json()
        except (httpx.HTTPError, ValueError) as exc:
            _logger.warning("geo lookup failed", extra={"public_ip": public_ip, "error": str(exc)})
            return None
        if payload.get("status") != "success":
            return None
        return GeoLookupResult(
            country=payload.get("country") or None,
            state=payload.get("regionName") or None,
            city=payload.get("city") or None,
            latitude=_float_or_none(payload.get("lat")),
            longitude=_float_or_none(payload.get("lon")),
            timezone=payload.get("timezone") or None,
            isp=payload.get("isp") or None,
            asn=payload.get("as") or None,
        )


def _float_or_none(value: object) -> float | None:
    if value is None:
        return None
    try:
        return float(value)
    except (TypeError, ValueError):
        return None
