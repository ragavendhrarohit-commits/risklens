from __future__ import annotations

import uuid
from datetime import datetime

from sqlalchemy import (
    BigInteger,
    Boolean,
    CheckConstraint,
    DateTime,
    ForeignKey,
    Index,
    Integer,
    MetaData,
    String,
    UniqueConstraint,
)
from sqlalchemy.dialects.postgresql import UUID as PGUUID
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column
from sqlalchemy.types import TypeDecorator

from app.domain.agent import Agent, AgentStatus
from app.domain.device import DeviceInventory
from app.domain.heartbeat import Heartbeat, HeartbeatRowStatus
from app.domain.applications import ApplicationActivity
from app.domain.browser import BrowserActivity
from app.domain.sessions import ComputerSession
from app.domain.software import InstalledApplication
from app.domain.user import UserAccount

# Deterministic constraint names so Alembic autogenerate produces stable diffs
# and ALTER statements can always target a known identifier.
NAMING_CONVENTION = {
    "ix": "ix_%(table_name)s_%(column_0_N_name)s",
    "uq": "uq_%(table_name)s_%(column_0_N_name)s",
    "ck": "ck_%(table_name)s_%(constraint_name)s",
    "fk": "fk_%(table_name)s_%(column_0_N_name)s_%(referred_table_name)s",
    "pk": "pk_%(table_name)s",
}


class Base(DeclarativeBase):
    metadata = MetaData(naming_convention=NAMING_CONVENTION)


class GUID(TypeDecorator):
    """UUID column that is native on PostgreSQL and CHAR(36) elsewhere.

    Keeps the model usable against SQLite for fast offline schema checks
    without weakening the PostgreSQL schema.
    """

    impl = String(36)
    cache_ok = True

    def load_dialect_impl(self, dialect):
        if dialect.name == "postgresql":
            return dialect.type_descriptor(PGUUID(as_uuid=True))
        return dialect.type_descriptor(String(36))

    def process_bind_param(self, value, dialect):
        if value is None:
            return None
        if not isinstance(value, uuid.UUID):
            value = uuid.UUID(str(value))
        if dialect.name == "postgresql":
            return value
        return str(value)

    def process_result_value(self, value, dialect):
        if value is None:
            return None
        if isinstance(value, uuid.UUID):
            return value
        return uuid.UUID(str(value))


class AgentRow(Base):
    """Persistence model for a registered agent.

    `agent_id` is the primary key, which is what enforces the uniqueness
    requirement: PostgreSQL backs a PRIMARY KEY with a unique index, so a
    second INSERT of the same AgentID raises IntegrityError. The repository
    translates that into the domain's DuplicateAgentError.
    """

    __tablename__ = "agents"

    agent_id: Mapped[uuid.UUID] = mapped_column(GUID(), primary_key=True)

    install_id: Mapped[str] = mapped_column(String(256), nullable=False)
    agent_version: Mapped[str] = mapped_column(String(64), nullable=False)
    machine_name: Mapped[str] = mapped_column(String(256), nullable=False)
    operating_system: Mapped[str] = mapped_column(String(256), nullable=False)
    display_name: Mapped[str | None] = mapped_column(String(256), nullable=True)

    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    registered_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)

    status: Mapped[str] = mapped_column(String(32), nullable=False)

    __table_args__ = (
        Index("ix_agents_install_id", "install_id"),
        Index("ix_agents_machine_name", "machine_name"),
        Index("ix_agents_status", "status"),
        Index("ix_agents_registered_at", "registered_at"),
        CheckConstraint("length(install_id) > 0", name="install_id_not_blank"),
        CheckConstraint("length(machine_name) > 0", name="machine_name_not_blank"),
    )

    def to_domain(self) -> Agent:
        return Agent(
            agent_id=self.agent_id,
            install_id=self.install_id,
            agent_version=self.agent_version,
            machine_name=self.machine_name,
            operating_system=self.operating_system,
            display_name=self.display_name,
            created_at=self.created_at,
            registered_at=self.registered_at,
            status=AgentStatus(self.status),
        )

    @classmethod
    def from_domain(cls, agent: Agent) -> "AgentRow":
        return cls(
            agent_id=agent.agent_id,
            install_id=agent.install_id,
            agent_version=agent.agent_version,
            machine_name=agent.machine_name,
            operating_system=agent.operating_system,
            display_name=agent.display_name,
            created_at=agent.created_at,
            registered_at=agent.registered_at,
            status=agent.status.value,
        )


class DeviceInventoryRow(Base):
    """Latest device inventory snapshot for one registered agent.

    `agent_id` is the primary key: upserts overwrite the previous row so the
    table always reflects the most recent upload. A foreign key to `agents`
    enforces that inventory can only be stored for registered agents at the
    database level as well as in the service layer.
    """

    __tablename__ = "device_inventory"

    agent_id: Mapped[uuid.UUID] = mapped_column(
        GUID(),
        ForeignKey("agents.agent_id", name="fk_device_inventory_agent_id_agents"),
        primary_key=True,
    )

    install_id: Mapped[str] = mapped_column(String(256), nullable=False)
    machine_name: Mapped[str] = mapped_column(String(256), nullable=False)
    hostname: Mapped[str] = mapped_column(String(256), nullable=False)
    operating_system: Mapped[str] = mapped_column(String(256), nullable=False)
    os_version: Mapped[str] = mapped_column(String(128), nullable=False, default="")
    windows_build: Mapped[str] = mapped_column(String(64), nullable=False, default="")
    architecture: Mapped[str] = mapped_column(String(64), nullable=False, default="")
    cpu_model: Mapped[str] = mapped_column(String(256), nullable=False, default="")
    cpu_cores: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    memory_bytes: Mapped[int] = mapped_column(BigInteger, nullable=False, default=0)
    disk_total_bytes: Mapped[int] = mapped_column(BigInteger, nullable=False, default=0)
    disk_free_bytes: Mapped[int] = mapped_column(BigInteger, nullable=False, default=0)
    manufacturer: Mapped[str] = mapped_column(String(256), nullable=False, default="")
    model: Mapped[str] = mapped_column(String(256), nullable=False, default="")
    serial_number: Mapped[str] = mapped_column(String(256), nullable=False, default="")
    boot_time: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    current_user: Mapped[str] = mapped_column(String(256), nullable=False, default="")
    agent_version: Mapped[str] = mapped_column(String(64), nullable=False)
    collected_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    received_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)

    __table_args__ = (
        Index("ix_device_inventory_machine_name", "machine_name"),
        Index("ix_device_inventory_hostname", "hostname"),
        Index("ix_device_inventory_received_at", "received_at"),
        CheckConstraint("cpu_cores >= 0", name="cpu_cores_non_negative"),
        CheckConstraint("memory_bytes >= 0", name="memory_bytes_non_negative"),
        CheckConstraint("disk_total_bytes >= 0", name="disk_total_bytes_non_negative"),
        CheckConstraint("disk_free_bytes >= 0", name="disk_free_bytes_non_negative"),
        CheckConstraint("length(install_id) > 0", name="install_id_not_blank"),
        CheckConstraint("length(machine_name) > 0", name="machine_name_not_blank"),
        CheckConstraint("length(hostname) > 0", name="hostname_not_blank"),
    )

    def to_domain(self) -> DeviceInventory:
        return DeviceInventory(
            agent_id=self.agent_id,
            install_id=self.install_id,
            machine_name=self.machine_name,
            hostname=self.hostname,
            operating_system=self.operating_system,
            os_version=self.os_version,
            windows_build=self.windows_build,
            architecture=self.architecture,
            cpu_model=self.cpu_model,
            cpu_cores=self.cpu_cores,
            memory_bytes=self.memory_bytes,
            disk_total_bytes=self.disk_total_bytes,
            disk_free_bytes=self.disk_free_bytes,
            manufacturer=self.manufacturer,
            model=self.model,
            serial_number=self.serial_number,
            boot_time=self.boot_time,
            current_user=self.current_user,
            agent_version=self.agent_version,
            collected_at=self.collected_at,
            received_at=self.received_at,
        )

    @classmethod
    def from_domain(cls, inventory: DeviceInventory) -> "DeviceInventoryRow":
        return cls(
            agent_id=inventory.agent_id,
            install_id=inventory.install_id,
            machine_name=inventory.machine_name,
            hostname=inventory.hostname,
            operating_system=inventory.operating_system,
            os_version=inventory.os_version,
            windows_build=inventory.windows_build,
            architecture=inventory.architecture,
            cpu_model=inventory.cpu_model,
            cpu_cores=inventory.cpu_cores,
            memory_bytes=inventory.memory_bytes,
            disk_total_bytes=inventory.disk_total_bytes,
            disk_free_bytes=inventory.disk_free_bytes,
            manufacturer=inventory.manufacturer,
            model=inventory.model,
            serial_number=inventory.serial_number,
            boot_time=inventory.boot_time,
            current_user=inventory.current_user,
            agent_version=inventory.agent_version,
            collected_at=inventory.collected_at,
            received_at=inventory.received_at,
        )


class AgentHeartbeatRow(Base):
    """Append-only heartbeat history for a registered agent."""

    __tablename__ = "agent_heartbeats"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    agent_id: Mapped[uuid.UUID] = mapped_column(
        GUID(),
        ForeignKey("agents.agent_id", name="fk_agent_heartbeats_agent_id_agents"),
        nullable=False,
    )
    heartbeat_time: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    received_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    status: Mapped[str] = mapped_column(String(32), nullable=False, default=HeartbeatRowStatus.ok.value)

    __table_args__ = (
        Index("ix_agent_heartbeats_agent_id", "agent_id"),
        Index("ix_agent_heartbeats_heartbeat_time", "heartbeat_time"),
        Index("ix_agent_heartbeats_agent_id_heartbeat_time", "agent_id", "heartbeat_time"),
    )

    def to_domain(self) -> Heartbeat:
        return Heartbeat(
            agent_id=self.agent_id,
            heartbeat_time=self.heartbeat_time,
            received_at=self.received_at,
            status=HeartbeatRowStatus(self.status),
        )

    @classmethod
    def from_domain(cls, heartbeat: Heartbeat) -> "AgentHeartbeatRow":
        return cls(
            agent_id=heartbeat.agent_id,
            heartbeat_time=heartbeat.heartbeat_time,
            received_at=heartbeat.received_at,
            status=heartbeat.status.value,
        )


class UserInventoryRow(Base):
    """Latest Windows user account inventory keyed by SID."""

    __tablename__ = "user_inventory"

    sid: Mapped[str] = mapped_column(String(256), primary_key=True)
    agent_id: Mapped[uuid.UUID] = mapped_column(
        GUID(),
        ForeignKey("agents.agent_id", name="fk_user_inventory_agent_id_agents"),
        nullable=False,
    )
    username: Mapped[str] = mapped_column(String(256), nullable=False)
    display_name: Mapped[str] = mapped_column(String(256), nullable=False, default="")
    account_type: Mapped[str] = mapped_column(String(64), nullable=False, default="Local")
    enabled: Mapped[bool] = mapped_column(Boolean, nullable=False, default=True)
    builtin: Mapped[bool] = mapped_column(Boolean, nullable=False, default=False)
    password_required: Mapped[bool] = mapped_column(Boolean, nullable=False, default=True)
    last_logon: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    source: Mapped[str] = mapped_column(String(64), nullable=False, default="Local")
    collected_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    received_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)

    __table_args__ = (
        Index("ix_user_inventory_agent_id", "agent_id"),
        Index("ix_user_inventory_username", "username"),
        Index("ix_user_inventory_account_type", "account_type"),
        Index("ix_user_inventory_enabled", "enabled"),
        CheckConstraint("length(sid) > 0", name="sid_not_blank"),
        CheckConstraint("length(username) > 0", name="username_not_blank"),
    )

    def to_domain(self) -> UserAccount:
        return UserAccount(
            sid=self.sid,
            agent_id=self.agent_id,
            username=self.username,
            display_name=self.display_name,
            account_type=self.account_type,
            enabled=self.enabled,
            builtin=self.builtin,
            password_required=self.password_required,
            last_logon=self.last_logon,
            source=self.source,
            collected_at=self.collected_at,
            received_at=self.received_at,
        )

    @classmethod
    def from_domain(cls, account: UserAccount) -> "UserInventoryRow":
        return cls(
            sid=account.sid,
            agent_id=account.agent_id,
            username=account.username,
            display_name=account.display_name,
            account_type=account.account_type,
            enabled=account.enabled,
            builtin=account.builtin,
            password_required=account.password_required,
            last_logon=account.last_logon,
            source=account.source,
            collected_at=account.collected_at,
            received_at=account.received_at,
        )


class SoftwareInventoryRow(Base):
    """Installed application row keyed by (agent_id, application_name, version)."""

    __tablename__ = "software_inventory"

    id: Mapped[str] = mapped_column(String(64), primary_key=True)
    agent_id: Mapped[uuid.UUID] = mapped_column(
        GUID(),
        ForeignKey("agents.agent_id", name="fk_software_inventory_agent_id_agents"),
        nullable=False,
    )
    application_name: Mapped[str] = mapped_column(String(512), nullable=False)
    version: Mapped[str] = mapped_column(String(128), nullable=False, default="")
    publisher: Mapped[str] = mapped_column(String(256), nullable=False, default="")
    install_date: Mapped[str] = mapped_column(String(32), nullable=False, default="")
    install_location: Mapped[str] = mapped_column(String(512), nullable=False, default="")
    estimated_size: Mapped[int] = mapped_column(BigInteger, nullable=False, default=0)
    architecture: Mapped[str] = mapped_column(String(16), nullable=False, default="")
    system_component: Mapped[bool] = mapped_column(Boolean, nullable=False, default=False)
    windows_component: Mapped[bool] = mapped_column(Boolean, nullable=False, default=False)
    registry_path: Mapped[str] = mapped_column(String(512), nullable=False, default="")
    collected_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    received_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)

    __table_args__ = (
        Index("ix_software_inventory_agent_id", "agent_id"),
        Index("ix_software_inventory_application_name", "application_name"),
        Index("ix_software_inventory_publisher", "publisher"),
        Index("ix_software_inventory_architecture", "architecture"),
        CheckConstraint("length(id) > 0", name="id_not_blank"),
        CheckConstraint("length(application_name) > 0", name="application_name_not_blank"),
        CheckConstraint("estimated_size >= 0", name="estimated_size_non_negative"),
    )

    def to_domain(self) -> InstalledApplication:
        return InstalledApplication(
            id=self.id,
            agent_id=self.agent_id,
            application_name=self.application_name,
            version=self.version,
            publisher=self.publisher,
            install_date=self.install_date,
            install_location=self.install_location,
            estimated_size=self.estimated_size,
            architecture=self.architecture,
            system_component=self.system_component,
            windows_component=self.windows_component,
            registry_path=self.registry_path,
            collected_at=self.collected_at,
            received_at=self.received_at,
        )

    @classmethod
    def from_domain(cls, app: InstalledApplication) -> "SoftwareInventoryRow":
        return cls(
            id=app.id,
            agent_id=app.agent_id,
            application_name=app.application_name,
            version=app.version,
            publisher=app.publisher,
            install_date=app.install_date,
            install_location=app.install_location,
            estimated_size=app.estimated_size,
            architecture=app.architecture,
            system_component=app.system_component,
            windows_component=app.windows_component,
            registry_path=app.registry_path,
            collected_at=app.collected_at,
            received_at=app.received_at,
        )


class PolicyRow(Base):
    """Catalog of agent collection policies. Read-only in Sprint 13."""

    __tablename__ = "policies"

    id: Mapped[uuid.UUID] = mapped_column(GUID(), primary_key=True)
    name: Mapped[str] = mapped_column(String(256), nullable=False)
    heartbeat_interval_seconds: Mapped[int] = mapped_column(Integer, nullable=False)
    collect_devices: Mapped[bool] = mapped_column(Boolean, nullable=False, default=True)
    collect_users: Mapped[bool] = mapped_column(Boolean, nullable=False, default=True)
    collect_software: Mapped[bool] = mapped_column(Boolean, nullable=False, default=True)
    collect_processes: Mapped[bool] = mapped_column(Boolean, nullable=False, default=False)
    collect_services: Mapped[bool] = mapped_column(Boolean, nullable=False, default=False)
    collect_event_logs: Mapped[bool] = mapped_column(Boolean, nullable=False, default=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)

    __table_args__ = (
        UniqueConstraint("name", name="uq_policies_name"),
        CheckConstraint("length(name) > 0", name="name_not_blank"),
        CheckConstraint("heartbeat_interval_seconds > 0", name="heartbeat_interval_positive"),
    )


class AgentPolicyRow(Base):
    """One assigned policy per registered agent."""

    __tablename__ = "agent_policy"

    agent_id: Mapped[uuid.UUID] = mapped_column(
        GUID(),
        ForeignKey("agents.agent_id", name="fk_agent_policy_agent_id_agents"),
        primary_key=True,
    )
    policy_id: Mapped[uuid.UUID] = mapped_column(
        GUID(),
        ForeignKey("policies.id", name="fk_agent_policy_policy_id_policies"),
        nullable=False,
    )
    assigned_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)

    __table_args__ = (Index("ix_agent_policy_policy_id", "policy_id"),)


class AgentTagRow(Base):
    """Optional operator tags on an agent. No seed assignments in Sprint 13."""

    __tablename__ = "agent_tags"

    agent_id: Mapped[uuid.UUID] = mapped_column(
        GUID(),
        ForeignKey("agents.agent_id", name="fk_agent_tags_agent_id_agents"),
        primary_key=True,
    )
    tag: Mapped[str] = mapped_column(String(64), primary_key=True)

    __table_args__ = (
        Index("ix_agent_tags_tag", "tag"),
        CheckConstraint("length(tag) > 0", name="tag_not_blank"),
        CheckConstraint(
            "tag IN ('Finance','HR','Production','Development','Servers','Lab')",
            name="allowed_tag",
        ),
    )


class AgentStatusRow(Base):
    """Current-state timestamps for Fleet. Does not copy inventory rows."""

    __tablename__ = "agent_status"

    agent_id: Mapped[uuid.UUID] = mapped_column(
        GUID(),
        ForeignKey("agents.agent_id", name="fk_agent_status_agent_id_agents"),
        primary_key=True,
    )
    last_seen: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    last_inventory_at: Mapped[datetime | None] = mapped_column(
        DateTime(timezone=True), nullable=True
    )
    last_communication_at: Mapped[datetime | None] = mapped_column(
        DateTime(timezone=True), nullable=True
    )
    ip_address: Mapped[str | None] = mapped_column(String(64), nullable=True)

    __table_args__ = (Index("ix_agent_status_last_seen", "last_seen"),)


class ComputerSessionRow(Base):
    """One Windows logon session on a managed computer."""

    __tablename__ = "computer_sessions"

    session_id: Mapped[uuid.UUID] = mapped_column(GUID(), primary_key=True)
    agent_id: Mapped[uuid.UUID] = mapped_column(
        GUID(),
        ForeignKey("agents.agent_id", name="fk_computer_sessions_agent_id_agents"),
        nullable=False,
    )
    username: Mapped[str] = mapped_column(String(256), nullable=False)
    account_sid: Mapped[str] = mapped_column(String(256), nullable=False, default="")
    logon_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    logoff_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    status: Mapped[str] = mapped_column(String(32), nullable=False)
    collected_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    received_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)

    __table_args__ = (
        Index("ix_computer_sessions_agent_id_logon_at", "agent_id", "logon_at"),
        Index("ix_computer_sessions_agent_id_status", "agent_id", "status"),
        CheckConstraint("length(username) > 0", name="username_not_blank"),
        CheckConstraint("status IN ('active','closed')", name="session_status_allowed"),
    )

    def to_domain(self) -> ComputerSession:
        return ComputerSession(
            session_id=self.session_id,
            agent_id=self.agent_id,
            username=self.username,
            account_sid=self.account_sid,
            logon_at=self.logon_at,
            logoff_at=self.logoff_at,
            status=self.status,
            collected_at=self.collected_at,
            received_at=self.received_at,
        )

    @classmethod
    def from_domain(cls, session: ComputerSession) -> "ComputerSessionRow":
        return cls(
            session_id=session.session_id,
            agent_id=session.agent_id,
            username=session.username,
            account_sid=session.account_sid,
            logon_at=session.logon_at,
            logoff_at=session.logoff_at,
            status=session.status,
            collected_at=session.collected_at,
            received_at=session.received_at,
        )


class ApplicationActivityRow(Base):
    """Foreground application usage belonging to one computer session."""

    __tablename__ = "application_activity"

    id: Mapped[str] = mapped_column(String(64), primary_key=True)
    session_id: Mapped[uuid.UUID] = mapped_column(
        GUID(),
        ForeignKey("computer_sessions.session_id", name="fk_application_activity_session_id_computer_sessions"),
        nullable=False,
    )
    agent_id: Mapped[uuid.UUID] = mapped_column(
        GUID(),
        ForeignKey("agents.agent_id", name="fk_application_activity_agent_id_agents"),
        nullable=False,
    )
    application_name: Mapped[str] = mapped_column(String(512), nullable=False)
    executable_name: Mapped[str] = mapped_column(String(512), nullable=False, default="")
    window_title: Mapped[str] = mapped_column(String(512), nullable=False, default="")
    started_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    stopped_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    duration_seconds: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    collected_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    received_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)

    __table_args__ = (
        Index("ix_application_activity_session_id_started_at", "session_id", "started_at"),
        Index("ix_application_activity_agent_id_started_at", "agent_id", "started_at"),
        CheckConstraint("length(id) > 0", name="activity_id_not_blank"),
        CheckConstraint("length(application_name) > 0", name="application_name_not_blank"),
        CheckConstraint("duration_seconds >= 0", name="duration_seconds_non_negative"),
    )

    def to_domain(self) -> ApplicationActivity:
        return ApplicationActivity(
            id=self.id,
            session_id=self.session_id,
            agent_id=self.agent_id,
            application_name=self.application_name,
            executable_name=self.executable_name,
            window_title=self.window_title,
            started_at=self.started_at,
            stopped_at=self.stopped_at,
            duration_seconds=self.duration_seconds,
            collected_at=self.collected_at,
            received_at=self.received_at,
        )

    @classmethod
    def from_domain(cls, activity: ApplicationActivity) -> "ApplicationActivityRow":
        return cls(
            id=activity.id,
            session_id=activity.session_id,
            agent_id=activity.agent_id,
            application_name=activity.application_name,
            executable_name=activity.executable_name,
            window_title=activity.window_title,
            started_at=activity.started_at,
            stopped_at=activity.stopped_at,
            duration_seconds=activity.duration_seconds,
            collected_at=activity.collected_at,
            received_at=activity.received_at,
        )


class BrowserActivityRow(Base):
    """Active-tab labels belonging to one computer session. Not a clock."""

    __tablename__ = "browser_activity"

    id: Mapped[str] = mapped_column(String(64), primary_key=True)
    session_id: Mapped[uuid.UUID] = mapped_column(
        GUID(),
        ForeignKey("computer_sessions.session_id", name="fk_browser_activity_session_id_computer_sessions"),
        nullable=False,
    )
    agent_id: Mapped[uuid.UUID] = mapped_column(
        GUID(),
        ForeignKey("agents.agent_id", name="fk_browser_activity_agent_id_agents"),
        nullable=False,
    )
    executable_name: Mapped[str] = mapped_column(String(512), nullable=False, default="")
    domain: Mapped[str] = mapped_column(String(256), nullable=False)
    url: Mapped[str] = mapped_column(String(512), nullable=False, default="")
    title: Mapped[str] = mapped_column(String(512), nullable=False, default="")
    profile: Mapped[str] = mapped_column(String(256), nullable=False, default="")
    window_id: Mapped[int] = mapped_column(BigInteger, nullable=False, default=0)
    tab_id: Mapped[int] = mapped_column(BigInteger, nullable=False, default=0)
    started_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    stopped_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    duration_seconds: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    collected_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    received_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)

    __table_args__ = (
        Index("ix_browser_activity_session_id_started_at", "session_id", "started_at"),
        Index("ix_browser_activity_agent_id_started_at", "agent_id", "started_at"),
        CheckConstraint("length(id) > 0", name="browser_activity_id_not_blank"),
        CheckConstraint("length(domain) > 0", name="browser_domain_not_blank"),
        CheckConstraint("duration_seconds >= 0", name="browser_duration_seconds_non_negative"),
    )

    def to_domain(self) -> BrowserActivity:
        return BrowserActivity(
            id=self.id,
            session_id=self.session_id,
            agent_id=self.agent_id,
            executable_name=self.executable_name,
            domain=self.domain,
            url=self.url,
            title=self.title,
            profile=self.profile,
            window_id=int(self.window_id),
            tab_id=int(self.tab_id),
            started_at=self.started_at,
            stopped_at=self.stopped_at,
            duration_seconds=self.duration_seconds,
            collected_at=self.collected_at,
            received_at=self.received_at,
        )

    @classmethod
    def from_domain(cls, activity: BrowserActivity) -> "BrowserActivityRow":
        return cls(
            id=activity.id,
            session_id=activity.session_id,
            agent_id=activity.agent_id,
            executable_name=activity.executable_name,
            domain=activity.domain,
            url=activity.url,
            title=activity.title,
            profile=activity.profile,
            window_id=activity.window_id,
            tab_id=activity.tab_id,
            started_at=activity.started_at,
            stopped_at=activity.stopped_at,
            duration_seconds=activity.duration_seconds,
            collected_at=activity.collected_at,
            received_at=activity.received_at,
        )
