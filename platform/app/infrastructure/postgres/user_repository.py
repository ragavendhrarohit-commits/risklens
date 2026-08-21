from __future__ import annotations

from sqlalchemy import Select, asc, desc, func, select
from sqlalchemy.dialects.postgresql import insert as pg_insert
from sqlalchemy.orm import Session, sessionmaker

from app.domain.user import ListUsersQuery, UserAccount, UserPage, UserRecord, parse_sort
from app.infrastructure.postgres.models import AgentRow, UserInventoryRow
from app.repositories.user_repository import UserRepository

_SORT_COLUMNS = {
    "username": UserInventoryRow.username,
    "display_name": UserInventoryRow.display_name,
    "sid": UserInventoryRow.sid,
    "account_type": UserInventoryRow.account_type,
    "enabled": UserInventoryRow.enabled,
    "last_logon": UserInventoryRow.last_logon,
    "machine_name": AgentRow.machine_name,
    "collected_at": UserInventoryRow.collected_at,
    "received_at": UserInventoryRow.received_at,
}


class PostgresUserRepository(UserRepository):
    def __init__(self, session_factory: sessionmaker[Session]) -> None:
        self._session_factory = session_factory

    def upsert_many(self, accounts: list[UserAccount]) -> int:
        if not accounts:
            return 0
        with self._session_factory() as session:
            dialect = session.bind.dialect.name if session.bind is not None else ""
            count = 0
            for account in accounts:
                row = UserInventoryRow.from_domain(account)
                values = {
                    column.name: getattr(row, column.name)
                    for column in UserInventoryRow.__table__.columns
                }
                if dialect == "postgresql":
                    stmt = pg_insert(UserInventoryRow).values(**values)
                    update_cols = {
                        key: stmt.excluded[key] for key in values if key != "sid"
                    }
                    stmt = stmt.on_conflict_do_update(
                        index_elements=[UserInventoryRow.sid],
                        set_=update_cols,
                    )
                    session.execute(stmt)
                else:
                    existing = session.get(UserInventoryRow, account.sid)
                    if existing is None:
                        session.add(row)
                    else:
                        for key, value in values.items():
                            if key != "sid":
                                setattr(existing, key, value)
                count += 1
            session.commit()
            return count

    def get(self, sid: str) -> UserRecord | None:
        with self._session_factory() as session:
            stmt = (
                select(UserInventoryRow, AgentRow.machine_name)
                .outerjoin(AgentRow, AgentRow.agent_id == UserInventoryRow.agent_id)
                .where(UserInventoryRow.sid == sid)
            )
            row = session.execute(stmt).first()
            if row is None:
                return None
            account, machine_name = row
            return _to_record(account, machine_name or "")

    def list(self, query: ListUsersQuery) -> UserPage:
        with self._session_factory() as session:
            base = select(UserInventoryRow, AgentRow.machine_name).outerjoin(
                AgentRow, AgentRow.agent_id == UserInventoryRow.agent_id
            )
            base = _apply_filters(base, query)
            count_stmt = select(func.count()).select_from(base.subquery())
            total = int(session.execute(count_stmt).scalar_one())

            field, descending = parse_sort(query.sort)
            column = _SORT_COLUMNS[field]
            order = desc(column) if descending else asc(column)
            stmt = (
                base.order_by(order, UserInventoryRow.sid)
                .offset(query.offset)
                .limit(query.limit)
            )
            items = [
                _to_record(account, machine_name or "")
                for account, machine_name in session.execute(stmt).all()
            ]
            return UserPage(
                items=items,
                total=total,
                limit=query.limit,
                offset=query.offset,
            )


def _apply_filters(stmt: Select, query: ListUsersQuery) -> Select:
    if query.username:
        stmt = stmt.where(UserInventoryRow.username.ilike(f"%{query.username}%"))
    if query.account_type:
        stmt = stmt.where(UserInventoryRow.account_type.ilike(f"%{query.account_type}%"))
    if query.enabled is not None:
        stmt = stmt.where(UserInventoryRow.enabled.is_(query.enabled))
    if query.agent_id is not None:
        stmt = stmt.where(UserInventoryRow.agent_id == query.agent_id)
    return stmt


def _to_record(row: UserInventoryRow, machine_name: str) -> UserRecord:
    return UserRecord(
        sid=row.sid,
        agent_id=row.agent_id,
        username=row.username,
        display_name=row.display_name,
        account_type=row.account_type,
        enabled=row.enabled,
        builtin=row.builtin,
        password_required=row.password_required,
        last_logon=row.last_logon,
        source=row.source,
        collected_at=row.collected_at,
        received_at=row.received_at,
        machine_name=machine_name,
    )
