from __future__ import annotations

from datetime import datetime
from typing import Annotated
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, StringConstraints

from app.domain.user import UpsertUsersCommand, UserAccountInput

NonEmptyStr = Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=256)]
BoundedStr = Annotated[str, StringConstraints(strip_whitespace=True, max_length=256)]
TypeStr = Annotated[str, StringConstraints(strip_whitespace=True, max_length=64)]


class UserAccountRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    username: NonEmptyStr
    sid: NonEmptyStr
    display_name: BoundedStr = ""
    account_type: TypeStr = "Local"
    enabled: bool = True
    builtin: bool = False
    password_required: bool = True
    last_logon: datetime | None = None
    source: TypeStr = "Local"


class UserInventoryRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    agent_id: UUID
    install_id: NonEmptyStr
    collected_at: datetime
    users: list[UserAccountRequest] = Field(default_factory=list)

    def to_command(self) -> UpsertUsersCommand:
        return UpsertUsersCommand(
            agent_id=self.agent_id,
            install_id=self.install_id,
            collected_at=self.collected_at,
            users=[
                UserAccountInput(
                    username=item.username,
                    sid=item.sid,
                    display_name=item.display_name,
                    account_type=item.account_type,
                    enabled=item.enabled,
                    builtin=item.builtin,
                    password_required=item.password_required,
                    last_logon=item.last_logon,
                    source=item.source,
                )
                for item in self.users
            ],
        )


class UserInventoryResponse(BaseModel):
    accepted: bool = True
    agent_id: UUID
    stored: int
    inventory_status: str = "stored"
    server_version: str
    server_time: datetime
    received_at: datetime


class UserItemResponse(BaseModel):
    sid: str
    agent_id: UUID
    username: str
    display_name: str
    account_type: str
    enabled: bool
    builtin: bool
    password_required: bool
    last_logon: datetime | None
    source: str
    collected_at: datetime
    received_at: datetime
    machine_name: str = ""


class UserListResponse(BaseModel):
    items: list[UserItemResponse]
    total: int
    limit: int
    offset: int
