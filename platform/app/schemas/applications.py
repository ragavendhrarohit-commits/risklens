from __future__ import annotations

from datetime import datetime
from typing import Annotated
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, StringConstraints

from app.domain.applications import ApplicationActivityInput, UpsertApplicationActivityCommand

NonEmptyStr = Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=256)]
NameStr = Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=512)]
BoundedStr = Annotated[str, StringConstraints(strip_whitespace=True, max_length=512)]
IdStr = Annotated[str, StringConstraints(strip_whitespace=True, max_length=64)]


class ApplicationActivityItemRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    session_id: UUID
    application_name: NameStr
    executable_name: BoundedStr = ""
    window_title: BoundedStr = ""
    started_at: datetime
    stopped_at: datetime | None = None
    duration_seconds: int = Field(default=0, ge=0)
    id: IdStr = ""


class ApplicationActivityUploadRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    agent_id: UUID
    install_id: NonEmptyStr
    collected_at: datetime
    activities: list[ApplicationActivityItemRequest] = Field(default_factory=list)

    def to_command(self) -> UpsertApplicationActivityCommand:
        return UpsertApplicationActivityCommand(
            agent_id=self.agent_id,
            install_id=self.install_id,
            collected_at=self.collected_at,
            activities=[
                ApplicationActivityInput(
                    session_id=item.session_id,
                    application_name=item.application_name,
                    executable_name=item.executable_name,
                    window_title=item.window_title,
                    started_at=item.started_at,
                    stopped_at=item.stopped_at,
                    duration_seconds=item.duration_seconds,
                    id=item.id,
                )
                for item in self.activities
            ],
        )


class ApplicationActivityUploadResponse(BaseModel):
    accepted: bool = True
    agent_id: UUID
    stored: int
    inventory_status: str = "stored"
    server_version: str
    server_time: datetime
    received_at: datetime


class ApplicationActivityItemResponse(BaseModel):
    id: str
    session_id: UUID
    agent_id: UUID
    application_name: str
    executable_name: str
    window_title: str
    started_at: datetime
    stopped_at: datetime | None
    duration_seconds: int
    collected_at: datetime
    received_at: datetime
    username: str = ""
    machine_name: str = ""


class ApplicationActivityListResponse(BaseModel):
    items: list[ApplicationActivityItemResponse]
    total: int
    limit: int
    offset: int
