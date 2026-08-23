from __future__ import annotations

from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, Query, status

from app.dependencies.container import Container, get_container
from app.domain.geolocation import ListGeolocationQuery
from app.schemas.errors import ErrorEnvelope
from app.schemas.geolocation import (
    GeolocationDetailResponse,
    GeolocationItemResponse,
    GeolocationListResponse,
    GeolocationSummaryResponse,
)
from app.services.geolocation_service import GeolocationNotFoundError, GeolocationService

router = APIRouter()


def get_geolocation_service(container: Container = Depends(get_container)) -> GeolocationService:
    return container.resolve(GeolocationService)


def _to_item(record) -> GeolocationItemResponse:
    return GeolocationItemResponse(
        agent_id=record.agent_id,
        computer=record.computer,
        user=record.user,
        hostname=record.hostname,
        status=record.status,
        city=record.city,
        state=record.state,
        country=record.country,
        public_ip=record.public_ip,
        private_ip=record.private_ip,
        isp=record.isp,
        timezone=record.timezone,
        latitude=record.latitude,
        longitude=record.longitude,
        asn=record.asn,
        last_seen=record.last_seen,
        operating_system=record.operating_system,
        online=record.online,
    )


@router.get(
    "",
    response_model=GeolocationListResponse,
    summary="List endpoint geolocations",
    description=(
        "Returns approximate IP-based locations for managed endpoints with search and filters."
    ),
)
def list_geolocations(
    q: str = Query(default="", description="Search computer, user, country, city, or public IP."),
    status: str = Query(default="", description="Filter by online or offline."),
    country: str = Query(default="", description="Filter by country."),
    city: str = Query(default="", description="Filter by city."),
    isp: str = Query(default="", description="Filter by ISP."),
    service: GeolocationService = Depends(get_geolocation_service),
) -> GeolocationListResponse:
    page = service.list(
        ListGeolocationQuery(
            q=q,
            status=status,
            country=country,
            city=city,
            isp=isp,
        )
    )
    return GeolocationListResponse(
        summary=GeolocationSummaryResponse(
            managed_devices=page.summary.managed_devices,
            online_devices=page.summary.online_devices,
            countries=page.summary.countries,
            cities=page.summary.cities,
        ),
        items=[_to_item(item) for item in page.items],
        total=page.total,
    )


@router.get(
    "/{agent_id}",
    response_model=GeolocationDetailResponse,
    summary="Get one endpoint geolocation",
    responses={404: {"model": ErrorEnvelope, "description": "Geolocation not found."}},
)
def get_geolocation(
    agent_id: UUID,
    service: GeolocationService = Depends(get_geolocation_service),
) -> GeolocationDetailResponse:
    try:
        record = service.get(agent_id)
    except GeolocationNotFoundError as exc:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(exc)) from exc
    return GeolocationDetailResponse(**_to_item(record).model_dump())
