from __future__ import annotations

from datetime import datetime, timezone
from typing import Any

from app.logging.setup import get_logger

_logger = get_logger("risklens.sessions.lifecycle")


def _now() -> str:
    return datetime.now(timezone.utc).isoformat()


def banner(title: str, **fields: Any) -> None:
    lines = ["------------------------------------------", title, f"Timestamp: {_now()}"]
    extra: dict[str, Any] = {"operation": title}
    for key, value in fields.items():
        label = key.replace("_", " ").title()
        lines.append(f"{label}: {value}")
        extra[key] = value
    lines.append("------------------------------------------")
    _logger.info("\n".join(lines), extra=extra)


def error(title: str, **fields: Any) -> None:
    lines = ["------------------------------------------", f"ERROR {title}", f"Timestamp: {_now()}"]
    extra: dict[str, Any] = {"operation": title}
    for key, value in fields.items():
        label = key.replace("_", " ").title()
        lines.append(f"{label}: {value}")
        extra[key] = value
    lines.append("------------------------------------------")
    _logger.error("\n".join(lines), extra=extra)


def fatal(title: str, **fields: Any) -> None:
    lines = ["------------------------------------------", f"FATAL {title}", f"Timestamp: {_now()}"]
    extra: dict[str, Any] = {"operation": title, "fatal": True}
    for key, value in fields.items():
        label = key.replace("_", " ").title()
        lines.append(f"{label}: {value}")
        extra[key] = value
    lines.append("------------------------------------------")
    _logger.critical("\n".join(lines), extra=extra)


def dump_query(title: str, agent_id: str, rows: list[dict]) -> None:
    sql = (
        "SELECT session_id, logon_at AS login_time, logoff_at AS logout_time, status "
        "FROM computer_sessions "
        f"WHERE agent_id = '{agent_id}' "
        "ORDER BY logon_at"
    )
    lines = [
        "==========================================",
        f"REPRO {title}",
        f"Timestamp: {_now()}",
        f"agent_id: {agent_id}",
        sql,
        "------------------------------------------",
    ]
    if not rows:
        lines.append("(no rows)")
    for row in rows:
        lines.append(
            f"session_id={row.get('session_id')}  "
            f"login_time={row.get('login_time')}  "
            f"logout_time={row.get('logout_time')}  "
            f"status={row.get('status')}"
        )
    actives = [row for row in rows if row.get("status") == "active"]
    lines.append(f"ACTIVE count: {len(actives)}")
    if len(actives) > 1:
        lines.append("TWO OR MORE ACTIVE SESSIONS PRESENT")
        lines.append(f"active_session_ids={[row.get('session_id') for row in actives]}")
    lines.append("==========================================")
    extra = {
        "operation": title,
        "agent_id": agent_id,
        "sql": sql,
        "rows": rows,
        "active_count": len(actives),
        "active_session_ids": [row.get("session_id") for row in actives],
    }
    if len(actives) > 1:
        _logger.critical("\n".join(lines), extra=extra)
        return
    _logger.info("\n".join(lines), extra=extra)
