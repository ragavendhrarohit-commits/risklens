from __future__ import annotations

from pydantic import BaseModel, Field


class RootResponse(BaseModel):
    service: str = Field(examples=["RiskLens Backend"])
    status: str = Field(examples=["running"])
    version: str = Field(examples=["0.1.0"])


class HealthResponse(BaseModel):
    status: str = Field(examples=["healthy"])


class VersionResponse(BaseModel):
    version: str = Field(examples=["0.1.0"])
    build_time: str = Field(examples=["2026-08-14T00:00:00Z"])
    environment: str = Field(examples=["development"])
