from __future__ import annotations

from datetime import datetime, timezone

from app.domain.agent import Agent, AgentStatus, RegisterAgentCommand
from app.logging.setup import get_logger
from app.repositories.agent_repository import AgentRepository

_logger = get_logger("risklens.agent")


class AgentRegistrationService:
    """Application service. Coordinates the register-agent use case.

    The service is deliberately thin: it constructs the Agent aggregate and
    hands it to the repository. Field-shape validation belongs to the API
    schema layer; uniqueness is enforced atomically by the repository.
    """

    def __init__(self, repository: AgentRepository, *, clock=None, projection=None) -> None:
        self._repo = repository
        self._clock = clock or _default_clock
        self._projection = projection

    def register(self, command: RegisterAgentCommand) -> Agent:
        agent = Agent(
            agent_id=command.agent_id,
            install_id=command.install_id.strip(),
            agent_version=command.agent_version.strip(),
            machine_name=command.machine_name.strip(),
            operating_system=command.operating_system.strip(),
            display_name=(command.display_name.strip() if command.display_name else None) or None,
            created_at=command.created_at,
            registered_at=self._clock(),
            status=AgentStatus.registered,
        )
        self._repo.add(agent)
        if self._projection is not None:
            self._projection.on_registered(agent.agent_id)
        _logger.info(
            "agent registered",
            extra={
                "agent_id": str(agent.agent_id),
                "install_id": agent.install_id,
                "agent_version": agent.agent_version,
                "machine_name": agent.machine_name,
                "operating_system": agent.operating_system,
            },
        )
        return agent


def _default_clock() -> datetime:
    return datetime.now(timezone.utc)
