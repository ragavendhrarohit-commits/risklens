from __future__ import annotations

from datetime import datetime
from typing import Annotated
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, StringConstraints

from app.domain.sessions import SessionInput, UpsertSessionsCommand

NonEmptyStr = Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=256)]
BoundedStr = Annotated[str, StringConstraints(strip_whitespace=True, max_length=256)]
StatusStr = Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=32)]


class SessionItemRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    session_id: UUID
    username: NonEmptyStr
    account_sid: BoundedStr = ""
    logon_at: datetime
    logoff_at: datetime | None = None
    status: StatusStr = "active"


class SessionUploadRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    agent_id: UUID
    install_id: NonEmptyStr
    collected_at: datetime
    sessions: list[SessionItemRequest] = Field(default_factory=list)

    def to_command(self) -> UpsertSessionsCommand:
        return UpsertSessionsCommand(
            agent_id=self.agent_id,
            install_id=self.install_id,
            collected_at=self.collected_at,
            sessions=[
                SessionInput(
                    session_id=item.session_id,
                    username=item.username,
                    account_sid=item.account_sid,
                    logon_at=item.logon_at,
                    logoff_at=item.logoff_at,
                    status=item.status,
                )
                for item in self.sessions
            ],
        )


class SessionUploadResponse(BaseModel):
    accepted: bool = True
    agent_id: UUID
    stored: int
    inventory_status: str = "stored"
    server_version: str
    server_time: datetime
    received_at: datetime


class SessionItemResponse(BaseModel):
    session_id: UUID
    agent_id: UUID
    username: str
    account_sid: str
    logon_at: datetime
    logoff_at: datetime | None
    status: str
    duration_seconds: int
    collected_at: datetime
    received_at: datetime
    machine_name: str = ""


class SessionListResponse(BaseModel):
    items: list[SessionItemResponse]
    total: int
    limit: int
    offset: int
