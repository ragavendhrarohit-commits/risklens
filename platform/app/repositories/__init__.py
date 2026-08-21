"""Repository interfaces (persistence ports). Adapters live under `app.infrastructure`."""

from app.repositories.agent_repository import AgentRepository

__all__ = ["AgentRepository"]
