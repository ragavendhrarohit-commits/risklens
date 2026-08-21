from __future__ import annotations

from datetime import datetime
from typing import Annotated
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, StringConstraints

from app.domain.software import SoftwareApplicationInput, UpsertSoftwareCommand

NonEmptyStr = Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=512)]
BoundedStr = Annotated[str, StringConstraints(strip_whitespace=True, max_length=512)]
ShortStr = Annotated[str, StringConstraints(strip_whitespace=True, max_length=128)]
TypeStr = Annotated[str, StringConstraints(strip_whitespace=True, max_length=16)]


class SoftwareApplicationRequest(BaseModel):
    model_config = ConfigDict(extra="forbid", populate_by_name=True)

    application_name: NonEmptyStr = Field(alias="application_name")
    version: ShortStr = ""
    publisher: BoundedStr = ""
    install_date: ShortStr = ""
    install_location: BoundedStr = ""
    estimated_size: int = Field(default=0, ge=0)
    architecture: TypeStr = ""
    system_component: bool = False
    windows_component: bool = False
    registry_path: BoundedStr = ""


class SoftwareInventoryRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    agent_id: UUID
    install_id: Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=256)]
    collected_at: datetime
    applications: list[SoftwareApplicationRequest] = Field(default_factory=list)

    def to_command(self) -> UpsertSoftwareCommand:
        return UpsertSoftwareCommand(
            agent_id=self.agent_id,
            install_id=self.install_id,
            collected_at=self.collected_at,
            applications=[
                SoftwareApplicationInput(
                    application_name=item.application_name,
                    version=item.version,
                    publisher=item.publisher,
                    install_date=item.install_date,
                    install_location=item.install_location,
                    estimated_size=item.estimated_size,
                    architecture=item.architecture,
                    system_component=item.system_component,
                    windows_component=item.windows_component,
                    registry_path=item.registry_path,
                )
                for item in self.applications
            ],
        )


class SoftwareInventoryResponse(BaseModel):
    accepted: bool = True
    agent_id: UUID
    stored: int
    inventory_status: str = "stored"
    server_version: str
    server_time: datetime
    received_at: datetime


class SoftwareItemResponse(BaseModel):
    id: str
    agent_id: UUID
    application_name: str
    version: str
    publisher: str
    install_date: str
    install_location: str
    estimated_size: int
    architecture: str
    system_component: bool
    windows_component: bool
    registry_path: str
    collected_at: datetime
    received_at: datetime
    machine_name: str = ""


class SoftwareListResponse(BaseModel):
    items: list[SoftwareItemResponse]
    total: int
    limit: int
    offset: int
