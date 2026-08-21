from __future__ import annotations

from datetime import datetime
from typing import Annotated
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, StringConstraints

from app.domain.browser import BrowserActivityInput, UpsertBrowserActivityCommand

NonEmptyStr = Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=256)]
BoundedStr = Annotated[str, StringConstraints(strip_whitespace=True, max_length=512)]
HostStr = Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=256)]
IdStr = Annotated[str, StringConstraints(strip_whitespace=True, max_length=64)]


class BrowserActivityItemRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    session_id: UUID
    executable_name: BoundedStr
    domain: HostStr
    url: BoundedStr = ""
    title: BoundedStr = ""
    profile: BoundedStr = ""
    window_id: int = 0
    tab_id: int = 0
    started_at: datetime
    stopped_at: datetime | None = None
    duration_seconds: int = Field(default=0, ge=0)
    id: IdStr = ""


class BrowserActivityUploadRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    agent_id: UUID
    install_id: NonEmptyStr
    collected_at: datetime
    activities: list[BrowserActivityItemRequest] = Field(default_factory=list)

    def to_command(self) -> UpsertBrowserActivityCommand:
        return UpsertBrowserActivityCommand(
            agent_id=self.agent_id,
            install_id=self.install_id,
            collected_at=self.collected_at,
            activities=[
                BrowserActivityInput(
                    session_id=item.session_id,
                    executable_name=item.executable_name,
                    domain=item.domain,
                    url=item.url,
                    title=item.title,
                    profile=item.profile,
                    window_id=item.window_id,
                    tab_id=item.tab_id,
                    started_at=item.started_at,
                    stopped_at=item.stopped_at,
                    duration_seconds=item.duration_seconds,
                    id=item.id,
                )
                for item in self.activities
            ],
        )


class BrowserActivityUploadResponse(BaseModel):
    accepted: bool = True
    agent_id: UUID
    stored: int
    inventory_status: str = "stored"
    server_version: str
    server_time: datetime
    received_at: datetime


class BrowserActivityItemResponse(BaseModel):
    id: str
    session_id: UUID
    agent_id: UUID
    executable_name: str
    domain: str
    url: str
    title: str
    profile: str
    window_id: int
    tab_id: int
    started_at: datetime
    stopped_at: datetime | None
    duration_seconds: int
    collected_at: datetime
    received_at: datetime


class BrowserActivityListResponse(BaseModel):
    items: list[BrowserActivityItemResponse]
    total: int
    limit: int
    offset: int
