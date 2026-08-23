from __future__ import annotations

from datetime import datetime, timedelta, timezone
from uuid import uuid4

from fastapi.testclient import TestClient

from app.domain.heartbeat import STALE_MAX_SECONDS
from app.domain.sessions import (
    SESSION_STATUS_ACTIVE,
    SESSION_STATUS_CLOSED,
    ComputerSession,
    ListSessionsQuery,
    cap_logoff_at,
    merge_fallback_close,
    merge_session,
)

REGISTER_URL = "/api/v1/agents/register"
SESSIONS_URL = "/api/v1/sessions"
APPLICATIONS_URL = "/api/v1/applications"
FLEET_URL = "/api/v1/fleet"

LOGIN_1 = datetime(2026, 8, 16, 8, 14, 25, tzinfo=timezone.utc)  # 18:14 AEST
LOGOUT_1 = datetime(2026, 8, 16, 11, 40, 0, tzinfo=timezone.utc)  # 21:40 AEST
LOGIN_2 = datetime(2026, 8, 16, 12, 18, 27, tzinfo=timezone.utc)  # 22:18 AEST
LOGOUT_2 = datetime(2026, 8, 16, 12, 30, 8, tzinfo=timezone.utc)  # 22:30 AEST
RECONCILE = datetime(2026, 8, 16, 12, 26, 8, tzinfo=timezone.utc)  # 22:26 AEST
LOGIN_EVE = LOGIN_2  # 22:18 AEST
LOGOUT_EVE = datetime(2026, 8, 16, 13, 14, 0, tzinfo=timezone.utc)  # 23:14 AEST
SHUTDOWN_EVE = datetime(2026, 8, 16, 13, 21, 0, tzinfo=timezone.utc)  # 23:21 AEST
LOGIN_NEXT = datetime(2026, 8, 16, 15, 20, 0, tzinfo=timezone.utc)  # 01:20 AEST next day
LOGOUT_NEXT = datetime(2026, 8, 16, 15, 45, 0, tzinfo=timezone.utc)  # 01:45 AEST next day


def _register(client: TestClient) -> str:
    agent_id = str(uuid4())
    response = client.post(
        REGISTER_URL,
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "agent_version": "0.1.0",
            "machine_name": "LAPTOP-B",
            "operating_system": "Windows 11",
            "created_at": LOGIN_1.isoformat(),
        },
    )
    assert response.status_code == 201, response.text
    return agent_id


def _heartbeat(client: TestClient, agent_id: str, when: datetime) -> None:
    response = client.post(
        f"/api/v1/agents/{agent_id}/heartbeat",
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "timestamp": when.isoformat(),
        },
    )
    assert response.status_code == 200, response.text


def _post_session(
    client: TestClient,
    agent_id: str,
    session_id: str,
    *,
    logon_at: datetime,
    logoff_at: datetime | None,
    status: str,
    collected_at: datetime | None = None,
) -> None:
    response = client.post(
        SESSIONS_URL,
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "collected_at": (collected_at or logon_at).isoformat(),
            "sessions": [
                {
                    "session_id": session_id,
                    "username": "ROHIT",
                    "account_sid": "S-1-5-21-1",
                    "logon_at": logon_at.isoformat(),
                    "logoff_at": None if logoff_at is None else logoff_at.isoformat(),
                    "status": status,
                }
            ],
        },
    )
    assert response.status_code == 200, response.text


def _get(client: TestClient, session_id: str) -> dict:
    response = client.get(f"{SESSIONS_URL}/{session_id}")
    assert response.status_code == 200, response.text
    return response.json()


def test_merge_does_not_reopen_or_replace_earlier_logout() -> None:
    sid = uuid4()
    agent = uuid4()
    closed = ComputerSession(
        session_id=sid,
        agent_id=agent,
        username="ROHIT",
        account_sid="S-1-5-21-1",
        logon_at=LOGIN_1,
        logoff_at=LOGOUT_1,
        status=SESSION_STATUS_CLOSED,
        collected_at=LOGOUT_1,
        received_at=LOGOUT_1,
    )
    later_close = ComputerSession(
        session_id=sid,
        agent_id=agent,
        username="ROHIT",
        account_sid="S-1-5-21-1",
        logon_at=LOGIN_1,
        logoff_at=RECONCILE,
        status=SESSION_STATUS_CLOSED,
        collected_at=RECONCILE,
        received_at=RECONCILE,
    )
    merged = merge_session(closed, later_close, LOGIN_2)
    assert merged.status == SESSION_STATUS_CLOSED
    assert merged.logoff_at == LOGOUT_1

    fallback = merge_fallback_close(closed, RECONCILE, RECONCILE, LOGIN_2)
    assert fallback is None

    latest_fallback = ComputerSession(
        session_id=sid,
        agent_id=agent,
        username="ROHIT",
        account_sid="S-1-5-21-1",
        logon_at=LOGIN_2,
        logoff_at=RECONCILE,
        status=SESSION_STATUS_CLOSED,
        collected_at=RECONCILE,
        received_at=RECONCILE,
    )
    real_shutdown = ComputerSession(
        session_id=sid,
        agent_id=agent,
        username="ROHIT",
        account_sid="S-1-5-21-1",
        logon_at=LOGIN_2,
        logoff_at=LOGOUT_2,
        status=SESSION_STATUS_CLOSED,
        collected_at=LOGOUT_2,
        received_at=LOGOUT_2,
    )
    assert merge_session(latest_fallback, real_shutdown).logoff_at == RECONCILE

    earlier_real = ComputerSession(
        session_id=sid,
        agent_id=agent,
        username="ROHIT",
        account_sid="S-1-5-21-1",
        logon_at=LOGIN_2,
        logoff_at=LOGOUT_1,
        status=SESSION_STATUS_CLOSED,
        collected_at=LOGOUT_1,
        received_at=LOGOUT_1,
    )
    assert merge_session(latest_fallback, earlier_real).logoff_at == RECONCILE

    reopen = ComputerSession(
        session_id=sid,
        agent_id=agent,
        username="ROHIT",
        account_sid="S-1-5-21-1",
        logon_at=LOGIN_1,
        logoff_at=None,
        status=SESSION_STATUS_ACTIVE,
        collected_at=LOGIN_2,
        received_at=LOGIN_2,
    )
    assert merge_session(closed, reopen).status == SESSION_STATUS_ACTIVE
    assert merge_session(closed, reopen).logoff_at is None
    assert cap_logoff_at(LOGIN_1, RECONCILE, LOGIN_2) == LOGIN_2


def test_missed_logout_then_second_login_matches_observed_bug(client: TestClient) -> None:
    """Session 1 logout missed; Session 2 login must close Session 1 before 22:26."""
    agent_id = _register(client)
    session_1 = str(uuid4())
    session_2 = str(uuid4())
    _heartbeat(client, agent_id, LOGIN_2)
    _post_session(client, agent_id, session_1, logon_at=LOGIN_1, logoff_at=None, status="active")
    _post_session(client, agent_id, session_2, logon_at=LOGIN_2, logoff_at=None, status="active")

    first = _get(client, session_1)
    second = _get(client, session_2)
    assert first["status"] == "closed"
    assert first["logoff_at"].startswith("2026-08-16T12:18:27")
    assert second["status"] == "active"
    assert second["logoff_at"] is None
    assert first["session_id"] != second["session_id"]

    late_real_logout = client.post(
        SESSIONS_URL,
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "collected_at": LOGOUT_1.isoformat(),
            "sessions": [
                {
                    "session_id": session_1,
                    "username": "ROHIT",
                    "account_sid": "S-1-5-21-1",
                    "logon_at": LOGIN_1.isoformat(),
                    "logoff_at": LOGOUT_1.isoformat(),
                    "status": "closed",
                }
            ],
        },
    )
    assert late_real_logout.status_code == 200
    corrected = _get(client, session_1)
    assert corrected["status"] == "closed"
    assert corrected["logoff_at"].startswith("2026-08-16T12:18:27")

    _post_session(
        client,
        agent_id,
        session_2,
        logon_at=LOGIN_2,
        logoff_at=LOGOUT_2,
        status="closed",
        collected_at=LOGOUT_2,
    )
    assert _get(client, session_2)["logoff_at"].startswith("2026-08-16T12:30:08")

    stale = datetime.now(timezone.utc) - timedelta(seconds=STALE_MAX_SECONDS + 30)
    _heartbeat(client, agent_id, stale)
    client.get(f"{FLEET_URL}/{agent_id}/sessions")
    assert _get(client, session_1)["logoff_at"].startswith("2026-08-16T12:18:27")
    assert _get(client, session_2)["logoff_at"].startswith("2026-08-16T12:30:08")


def test_login_logout(client: TestClient) -> None:
    agent_id = _register(client)
    session_id = str(uuid4())
    _post_session(client, agent_id, session_id, logon_at=LOGIN_1, logoff_at=None, status="active")
    _post_session(
        client,
        agent_id,
        session_id,
        logon_at=LOGIN_1,
        logoff_at=LOGOUT_1,
        status="closed",
        collected_at=LOGOUT_1,
    )
    body = _get(client, session_id)
    assert body["status"] == "closed"
    assert body["duration_seconds"] == int((LOGOUT_1 - LOGIN_1).total_seconds())


def test_login_shutdown(client: TestClient) -> None:
    agent_id = _register(client)
    session_id = str(uuid4())
    _heartbeat(client, agent_id, datetime.now(timezone.utc))
    _post_session(client, agent_id, session_id, logon_at=LOGIN_2, logoff_at=None, status="active")
    _post_session(
        client,
        agent_id,
        session_id,
        logon_at=LOGIN_2,
        logoff_at=LOGOUT_2,
        status="closed",
        collected_at=LOGOUT_2,
    )
    body = _get(client, session_id)
    assert body["status"] == "closed"
    fleet = client.get(f"{FLEET_URL}/{agent_id}")
    assert fleet.json()["online_status"] == "online"


def test_duplicate_logout_is_idempotent(client: TestClient) -> None:
    agent_id = _register(client)
    session_id = str(uuid4())
    _post_session(
        client,
        agent_id,
        session_id,
        logon_at=LOGIN_1,
        logoff_at=LOGOUT_1,
        status="closed",
        collected_at=LOGOUT_1,
    )
    _post_session(
        client,
        agent_id,
        session_id,
        logon_at=LOGIN_1,
        logoff_at=LOGOUT_1,
        status="closed",
        collected_at=LOGOUT_1,
    )
    listed = client.get(SESSIONS_URL, params={"agent_id": agent_id})
    assert listed.json()["total"] == 1
    assert _get(client, session_id)["logoff_at"].startswith("2026-08-16T11:40:00")


def test_active_refresh_reopens_closed_session_for_same_logon(client: TestClient) -> None:
    """Agent ACTIVE heartbeats for the same Windows logon reopen a falsely closed session."""
    agent_id = _register(client)
    session_id = str(uuid4())
    _post_session(
        client,
        agent_id,
        session_id,
        logon_at=LOGIN_1,
        logoff_at=LOGOUT_1,
        status="closed",
        collected_at=LOGOUT_1,
    )
    _post_session(client, agent_id, session_id, logon_at=LOGIN_1, logoff_at=None, status="active")
    body = _get(client, session_id)
    assert body["status"] == "active"
    assert body["logoff_at"] is None


def test_consecutive_logins_never_overlap(client: TestClient) -> None:
    agent_id = _register(client)
    first = str(uuid4())
    second = str(uuid4())
    _post_session(client, agent_id, first, logon_at=LOGIN_1, logoff_at=None, status="active")
    _post_session(client, agent_id, second, logon_at=LOGIN_2, logoff_at=None, status="active")
    items = client.get(SESSIONS_URL, params={"agent_id": agent_id}).json()["items"]
    by_id = {item["session_id"]: item for item in items}
    assert by_id[first]["status"] == "closed"
    assert by_id[second]["status"] == "active"
    assert by_id[first]["logoff_at"] <= by_id[second]["logon_at"]


def test_heartbeat_timeout_does_not_overwrite_real_logout(client: TestClient) -> None:
    agent_id = _register(client)
    session_id = str(uuid4())
    _post_session(
        client,
        agent_id,
        session_id,
        logon_at=LOGIN_1,
        logoff_at=LOGOUT_1,
        status="closed",
        collected_at=LOGOUT_1,
    )
    stale = datetime.now(timezone.utc) - timedelta(seconds=STALE_MAX_SECONDS + 60)
    _heartbeat(client, agent_id, stale)
    client.get(f"{FLEET_URL}/{agent_id}/sessions")
    body = _get(client, session_id)
    assert body["status"] == "closed"
    assert body["logoff_at"].startswith("2026-08-16T11:40:00")


def test_login_restart_uploads_closed_session(client: TestClient) -> None:
    agent_id = _register(client)
    session_id = str(uuid4())
    _post_session(client, agent_id, session_id, logon_at=LOGIN_2, logoff_at=None, status="active")
    _post_session(
        client,
        agent_id,
        session_id,
        logon_at=LOGIN_2,
        logoff_at=LOGOUT_2,
        status="closed",
        collected_at=LOGOUT_2,
    )
    body = _get(client, session_id)
    assert body["status"] == "closed"
    assert body["duration_seconds"] == int((LOGOUT_2 - LOGIN_2).total_seconds())


def test_sleep_wake_does_not_close_session(client: TestClient) -> None:
    agent_id = _register(client)
    session_id = str(uuid4())
    _post_session(client, agent_id, session_id, logon_at=LOGIN_1, logoff_at=None, status="active")
    _post_session(client, agent_id, session_id, logon_at=LOGIN_1, logoff_at=None, status="active")
    body = _get(client, session_id)
    assert body["status"] == "active"
    assert body["logoff_at"] is None


def test_agent_restart_keeps_open_session_until_windows_logoff(client: TestClient) -> None:
    agent_id = _register(client)
    session_id = str(uuid4())
    _post_session(client, agent_id, session_id, logon_at=LOGIN_1, logoff_at=None, status="active")
    _post_session(client, agent_id, session_id, logon_at=LOGIN_1, logoff_at=None, status="active")
    body = _get(client, session_id)
    assert body["status"] == "active"
    listed = client.get(SESSIONS_URL, params={"agent_id": agent_id}).json()
    assert listed["total"] == 1


def test_offline_fallback_then_real_logout_keeps_earlier_timestamp(client: TestClient) -> None:
    agent_id = _register(client)
    session_id = str(uuid4())
    _post_session(client, agent_id, session_id, logon_at=LOGIN_2, logoff_at=None, status="active")
    stale = datetime.now(timezone.utc) - timedelta(seconds=STALE_MAX_SECONDS + 30)
    _heartbeat(client, agent_id, stale)
    client.get(f"{FLEET_URL}/{agent_id}/sessions")
    fallback = _get(client, session_id)
    assert fallback["status"] == "closed"
    fallback_at = datetime.fromisoformat(fallback["logoff_at"].replace("Z", "+00:00"))
    real = fallback_at - timedelta(minutes=7)
    _post_session(
        client,
        agent_id,
        session_id,
        logon_at=LOGIN_2,
        logoff_at=real,
        status="closed",
        collected_at=real,
    )
    body = _get(client, session_id)
    assert body["status"] == "closed"
    assert datetime.fromisoformat(body["logoff_at"].replace("Z", "+00:00")) == fallback_at
    later = fallback_at + timedelta(minutes=5)
    _post_session(
        client,
        agent_id,
        session_id,
        logon_at=LOGIN_2,
        logoff_at=later,
        status="closed",
        collected_at=later,
    )
    still = _get(client, session_id)
    assert datetime.fromisoformat(still["logoff_at"].replace("Z", "+00:00")) == fallback_at
    listed = client.get(SESSIONS_URL, params={"agent_id": agent_id}).json()
    assert listed["total"] == 1


def test_scenario_login_logout_login_logout_does_not_fabricate_login(client: TestClient) -> None:
    """22:18 login, 23:14 logout, 01:20 login, 01:45 logout. No session starts at 23:14."""
    agent_id = _register(client)
    first = str(uuid4())
    second = str(uuid4())
    _post_session(client, agent_id, first, logon_at=LOGIN_EVE, logoff_at=None, status="active")
    _post_session(
        client,
        agent_id,
        first,
        logon_at=LOGIN_EVE,
        logoff_at=LOGOUT_EVE,
        status="closed",
        collected_at=LOGOUT_EVE,
    )
    _post_session(client, agent_id, second, logon_at=LOGIN_NEXT, logoff_at=None, status="active")
    _post_session(
        client,
        agent_id,
        second,
        logon_at=LOGIN_NEXT,
        logoff_at=LOGOUT_NEXT,
        status="closed",
        collected_at=LOGOUT_NEXT,
    )
    listed = client.get(SESSIONS_URL, params={"agent_id": agent_id}).json()
    assert listed["total"] == 2
    by_id = {row["session_id"]: row for row in listed["items"]}
    assert by_id[first]["logon_at"].startswith("2026-08-16T12:18:27")
    assert by_id[first]["logoff_at"].startswith("2026-08-16T13:14:00")
    assert by_id[second]["logon_at"].startswith("2026-08-16T15:20:00")
    assert by_id[second]["logoff_at"].startswith("2026-08-16T15:45:00")
    for row in listed["items"]:
        assert not row["logon_at"].startswith("2026-08-16T13:14:00")


def test_scenario_power_failure_closes_previous_at_next_login(client: TestClient) -> None:
    """22:18 login, power failure, 01:20 login. Session 1 ends at 01:20; session 2 is active."""
    agent_id = _register(client)
    first = str(uuid4())
    second = str(uuid4())
    _post_session(client, agent_id, first, logon_at=LOGIN_EVE, logoff_at=None, status="active")
    _post_session(
        client,
        agent_id,
        first,
        logon_at=LOGIN_EVE,
        logoff_at=LOGIN_NEXT,
        status="closed",
        collected_at=LOGIN_NEXT,
    )
    _post_session(client, agent_id, second, logon_at=LOGIN_NEXT, logoff_at=None, status="active")
    listed = client.get(SESSIONS_URL, params={"agent_id": agent_id}).json()
    assert listed["total"] == 2
    by_id = {row["session_id"]: row for row in listed["items"]}
    assert by_id[first]["status"] == "closed"
    assert by_id[first]["logoff_at"].startswith("2026-08-16T15:20:00")
    assert by_id[second]["status"] == "active"
    assert by_id[second]["logon_at"].startswith("2026-08-16T15:20:00")
    assert by_id[second]["logoff_at"] is None
    assert sum(1 for row in listed["items"] if row["status"] == "active") == 1


def test_scenario_shutdown_closes_only_the_real_login(client: TestClient) -> None:
    """22:18 login then shutdown. Only that session exists; it ends at shutdown."""
    agent_id = _register(client)
    session_id = str(uuid4())
    _post_session(client, agent_id, session_id, logon_at=LOGIN_EVE, logoff_at=None, status="active")
    _post_session(
        client,
        agent_id,
        session_id,
        logon_at=LOGIN_EVE,
        logoff_at=SHUTDOWN_EVE,
        status="closed",
        collected_at=SHUTDOWN_EVE,
    )
    listed = client.get(SESSIONS_URL, params={"agent_id": agent_id}).json()
    assert listed["total"] == 1
    body = listed["items"][0]
    assert body["session_id"] == session_id
    assert body["logon_at"].startswith("2026-08-16T12:18:27")
    assert body["logoff_at"].startswith("2026-08-16T13:21:00")
    assert body["status"] == "closed"


def test_next_login_fallback_does_not_replace_real_logout(client: TestClient) -> None:
    """Next-login close is a fallback only. It must not overwrite 23:14 with 01:20."""
    agent_id = _register(client)
    first = str(uuid4())
    second = str(uuid4())
    _post_session(client, agent_id, first, logon_at=LOGIN_EVE, logoff_at=None, status="active")
    _post_session(
        client,
        agent_id,
        first,
        logon_at=LOGIN_EVE,
        logoff_at=LOGOUT_EVE,
        status="closed",
        collected_at=LOGOUT_EVE,
    )
    _post_session(
        client,
        agent_id,
        first,
        logon_at=LOGIN_EVE,
        logoff_at=LOGIN_NEXT,
        status="closed",
        collected_at=LOGIN_NEXT,
    )
    _post_session(client, agent_id, second, logon_at=LOGIN_NEXT, logoff_at=None, status="active")
    listed = client.get(SESSIONS_URL, params={"agent_id": agent_id}).json()
    by_id = {row["session_id"]: row for row in listed["items"]}
    assert by_id[first]["logoff_at"].startswith("2026-08-16T13:14:00")
    assert by_id[second]["logon_at"].startswith("2026-08-16T15:20:00")
    assert not by_id[second]["logon_at"].startswith("2026-08-16T13:14:00")


def test_queued_logout_cannot_rewrite_closed_shutdown(client: TestClient) -> None:
    """Once shutdown closed the session, a queued earlier or later logout is ignored."""
    agent_id = _register(client)
    session_id = str(uuid4())
    _post_session(client, agent_id, session_id, logon_at=LOGIN_EVE, logoff_at=None, status="active")
    _post_session(
        client,
        agent_id,
        session_id,
        logon_at=LOGIN_EVE,
        logoff_at=SHUTDOWN_EVE,
        status="closed",
        collected_at=SHUTDOWN_EVE,
    )
    _post_session(
        client,
        agent_id,
        session_id,
        logon_at=LOGIN_EVE,
        logoff_at=LOGOUT_EVE,
        status="closed",
        collected_at=LOGOUT_EVE,
    )
    body = _get(client, session_id)
    assert body["logoff_at"].startswith("2026-08-16T13:21:00")
    _post_session(
        client,
        agent_id,
        session_id,
        logon_at=LOGIN_EVE,
        logoff_at=SHUTDOWN_EVE + timedelta(minutes=14),
        status="closed",
        collected_at=SHUTDOWN_EVE + timedelta(minutes=14),
    )
    still = _get(client, session_id)
    assert still["logoff_at"].startswith("2026-08-16T13:21:00")
    assert still["status"] == "closed"


def test_consecutive_login_logout_pairs(client: TestClient) -> None:
    agent_id = _register(client)
    first = str(uuid4())
    second = str(uuid4())
    _post_session(client, agent_id, first, logon_at=LOGIN_1, logoff_at=None, status="active")
    _post_session(
        client,
        agent_id,
        first,
        logon_at=LOGIN_1,
        logoff_at=LOGOUT_1,
        status="closed",
        collected_at=LOGOUT_1,
    )
    _post_session(client, agent_id, second, logon_at=LOGIN_2, logoff_at=None, status="active")
    _post_session(
        client,
        agent_id,
        second,
        logon_at=LOGIN_2,
        logoff_at=LOGOUT_2,
        status="closed",
        collected_at=LOGOUT_2,
    )
    items = client.get(SESSIONS_URL, params={"agent_id": agent_id, "sort": "logon_at"}).json()["items"]
    assert len(items) == 2
    assert items[0]["session_id"] == first
    assert items[1]["session_id"] == second
    assert items[0]["logoff_at"].startswith("2026-08-16T11:40:00")
    assert items[1]["logoff_at"].startswith("2026-08-16T12:30:08")
    assert items[0]["logoff_at"] <= items[1]["logon_at"]
    assert items[0]["duration_seconds"] == int((LOGOUT_1 - LOGIN_1).total_seconds())
    assert items[1]["duration_seconds"] == int((LOGOUT_2 - LOGIN_2).total_seconds())
    actives = [item for item in items if item["status"] == "active"]
    assert actives == []


def test_exact_observed_timeline(client: TestClient) -> None:
    """Reproduce 18:14 / ~21:40 then 22:18 / 22:30. Offline must not rewrite either logout."""
    agent_id = _register(client)
    session_1 = str(uuid4())
    session_2 = str(uuid4())
    _post_session(client, agent_id, session_1, logon_at=LOGIN_1, logoff_at=None, status="active")
    _post_session(client, agent_id, session_2, logon_at=LOGIN_2, logoff_at=None, status="active")

    first = _get(client, session_1)
    assert first["status"] == "closed"
    assert first["logoff_at"].startswith("2026-08-16T12:18:27")
    assert _get(client, session_2)["status"] == "active"

    _post_session(
        client,
        agent_id,
        session_1,
        logon_at=LOGIN_1,
        logoff_at=LOGOUT_1,
        status="closed",
        collected_at=LOGOUT_1,
    )
    _post_session(
        client,
        agent_id,
        session_2,
        logon_at=LOGIN_2,
        logoff_at=LOGOUT_2,
        status="closed",
        collected_at=LOGOUT_2,
    )
    first = _get(client, session_1)
    second = _get(client, session_2)
    assert first["logoff_at"].startswith("2026-08-16T12:18:27")
    assert second["logoff_at"].startswith("2026-08-16T12:30:08")
    assert first["duration_seconds"] == int((LOGIN_2 - LOGIN_1).total_seconds())
    assert second["duration_seconds"] == int((LOGOUT_2 - LOGIN_2).total_seconds())

    stale = datetime.now(timezone.utc) - timedelta(seconds=STALE_MAX_SECONDS + 30)
    _heartbeat(client, agent_id, stale)
    client.get(f"{FLEET_URL}/{agent_id}/sessions")
    assert _get(client, session_1)["logoff_at"].startswith("2026-08-16T12:18:27")
    assert _get(client, session_2)["logoff_at"].startswith("2026-08-16T12:30:08")
    listed = client.get(SESSIONS_URL, params={"agent_id": agent_id}).json()
    assert listed["total"] == 2
    actives = [item for item in listed["items"] if item["status"] == "active"]
    assert actives == []


def _active_count(client: TestClient, agent_id: str) -> int:
    response = client.get(SESSIONS_URL, params={"agent_id": agent_id, "status": "active"})
    assert response.status_code == 200, response.text
    body = response.json()
    assert body["total"] <= 1
    return body["total"]


def test_login_then_login_then_shutdown_closes_only_current(client: TestClient) -> None:
    """Login A, login B, shutdown must not stamp both rows with the shutdown time."""
    login_a = datetime(2026, 8, 16, 12, 18, 27, tzinfo=timezone.utc)  # 22:18 AEST
    login_b = datetime(2026, 8, 16, 13, 14, 0, tzinfo=timezone.utc)  # 23:14 AEST
    shutdown_at = datetime(2026, 8, 16, 13, 21, 0, tzinfo=timezone.utc)  # 23:21 AEST
    agent_id = _register(client)
    session_a = str(uuid4())
    session_b = str(uuid4())

    _post_session(client, agent_id, session_a, logon_at=login_a, logoff_at=None, status="active")
    assert _active_count(client, agent_id) == 1

    _post_session(client, agent_id, session_b, logon_at=login_b, logoff_at=None, status="active")
    first = _get(client, session_a)
    second = _get(client, session_b)
    assert first["status"] == "closed"
    assert first["logoff_at"].startswith("2026-08-16T13:14:00")
    assert first["duration_seconds"] == int((login_b - login_a).total_seconds())
    assert second["status"] == "active"
    assert second["logoff_at"] is None
    assert _active_count(client, agent_id) == 1

    _post_session(
        client,
        agent_id,
        session_b,
        logon_at=login_b,
        logoff_at=shutdown_at,
        status="closed",
        collected_at=shutdown_at,
    )
    first_after = _get(client, session_a)
    second_after = _get(client, session_b)
    assert first_after["logoff_at"].startswith("2026-08-16T13:14:00")
    assert first_after["received_at"] == first["received_at"]
    assert second_after["status"] == "closed"
    assert second_after["logoff_at"].startswith("2026-08-16T13:21:00")
    assert second_after["duration_seconds"] == int((shutdown_at - login_b).total_seconds())
    assert _active_count(client, agent_id) == 0


def test_shutdown_payload_cannot_rewrite_historical_session(client: TestClient) -> None:
    login_a = datetime(2026, 8, 16, 12, 18, 27, tzinfo=timezone.utc)
    login_b = datetime(2026, 8, 16, 13, 14, 0, tzinfo=timezone.utc)
    shutdown_at = datetime(2026, 8, 16, 13, 21, 0, tzinfo=timezone.utc)
    agent_id = _register(client)
    session_a = str(uuid4())
    session_b = str(uuid4())
    _post_session(client, agent_id, session_a, logon_at=login_a, logoff_at=None, status="active")
    _post_session(client, agent_id, session_b, logon_at=login_b, logoff_at=None, status="active")
    before = _get(client, session_a)

    response = client.post(
        SESSIONS_URL,
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "collected_at": shutdown_at.isoformat(),
            "sessions": [
                {
                    "session_id": session_a,
                    "username": "ROHIT",
                    "account_sid": "S-1-5-21-1",
                    "logon_at": login_a.isoformat(),
                    "logoff_at": shutdown_at.isoformat(),
                    "status": "closed",
                },
                {
                    "session_id": session_b,
                    "username": "ROHIT",
                    "account_sid": "S-1-5-21-1",
                    "logon_at": login_b.isoformat(),
                    "logoff_at": shutdown_at.isoformat(),
                    "status": "closed",
                },
            ],
        },
    )
    assert response.status_code == 200, response.text
    after_a = _get(client, session_a)
    after_b = _get(client, session_b)
    assert after_a["logoff_at"].startswith("2026-08-16T13:14:00")
    assert after_a["received_at"] == before["received_at"]
    assert after_b["logoff_at"].startswith("2026-08-16T13:21:00")
    assert _active_count(client, agent_id) == 0


def test_duplicate_login_does_not_create_second_session(client: TestClient) -> None:
    agent_id = _register(client)
    session_id = str(uuid4())
    _post_session(client, agent_id, session_id, logon_at=LOGIN_1, logoff_at=None, status="active")
    _post_session(client, agent_id, session_id, logon_at=LOGIN_1, logoff_at=None, status="active")
    listed = client.get(SESSIONS_URL, params={"agent_id": agent_id}).json()
    assert listed["total"] == 1
    assert listed["items"][0]["session_id"] == session_id
    assert listed["items"][0]["status"] == "active"
    assert _active_count(client, agent_id) == 1


def test_duplicate_shutdown_is_idempotent(client: TestClient) -> None:
    login_b = datetime(2026, 8, 16, 13, 14, 0, tzinfo=timezone.utc)
    shutdown_at = datetime(2026, 8, 16, 13, 21, 0, tzinfo=timezone.utc)
    agent_id = _register(client)
    session_id = str(uuid4())
    _post_session(client, agent_id, session_id, logon_at=login_b, logoff_at=None, status="active")
    _post_session(
        client,
        agent_id,
        session_id,
        logon_at=login_b,
        logoff_at=shutdown_at,
        status="closed",
        collected_at=shutdown_at,
    )
    first = _get(client, session_id)
    _post_session(
        client,
        agent_id,
        session_id,
        logon_at=login_b,
        logoff_at=shutdown_at,
        status="closed",
        collected_at=shutdown_at,
    )
    second = _get(client, session_id)
    assert second["status"] == "closed"
    assert second["logoff_at"] == first["logoff_at"]
    assert second["received_at"] == first["received_at"]
    listed = client.get(SESSIONS_URL, params={"agent_id": agent_id}).json()
    assert listed["total"] == 1
    assert _active_count(client, agent_id) == 0


def test_offline_reconciliation_only_closes_active_session(client: TestClient) -> None:
    login_a = datetime(2026, 8, 16, 12, 18, 27, tzinfo=timezone.utc)
    login_b = datetime(2026, 8, 16, 13, 14, 0, tzinfo=timezone.utc)
    agent_id = _register(client)
    session_a = str(uuid4())
    session_b = str(uuid4())
    _post_session(client, agent_id, session_a, logon_at=login_a, logoff_at=None, status="active")
    _post_session(client, agent_id, session_b, logon_at=login_b, logoff_at=None, status="active")
    historical = _get(client, session_a)
    assert historical["status"] == "closed"

    stale = datetime.now(timezone.utc) - timedelta(seconds=STALE_MAX_SECONDS + 30)
    _heartbeat(client, agent_id, stale)
    client.get(f"{FLEET_URL}/{agent_id}/sessions")

    after_a = _get(client, session_a)
    after_b = _get(client, session_b)
    assert after_a["logoff_at"] == historical["logoff_at"]
    assert after_a["received_at"] == historical["received_at"]
    assert after_a["status"] == "closed"
    assert after_b["status"] == "closed"
    assert after_b["logoff_at"] is not None
    assert _active_count(client, agent_id) == 0


def test_shutdown_logout_is_immutable_after_later_login(client: TestClient) -> None:
    """Session A 12:22 -> shutdown 12:25 must never become 12:35 after Session B 12:34."""
    login_a = datetime(2026, 8, 16, 12, 22, 56, tzinfo=timezone.utc)
    shutdown_at = datetime(2026, 8, 16, 12, 25, 25, tzinfo=timezone.utc)
    login_b = datetime(2026, 8, 16, 12, 34, 19, tzinfo=timezone.utc)
    stale_close = datetime(2026, 8, 16, 12, 35, 34, tzinfo=timezone.utc)
    agent_id = _register(client)
    session_a = str(uuid4())
    session_b = str(uuid4())

    _post_session(client, agent_id, session_a, logon_at=login_a, logoff_at=None, status="active")
    _post_session(
        client,
        agent_id,
        session_a,
        logon_at=login_a,
        logoff_at=shutdown_at,
        status="closed",
        collected_at=shutdown_at,
    )
    closed = _get(client, session_a)
    assert closed["status"] == "closed"
    assert closed["logoff_at"].startswith("2026-08-16T12:25:25")
    duration = closed["duration_seconds"]
    received = closed["received_at"]
    assert duration == int((shutdown_at - login_a).total_seconds())

    _post_session(client, agent_id, session_b, logon_at=login_b, logoff_at=None, status="active")
    after_login = _get(client, session_a)
    assert after_login["logoff_at"] == closed["logoff_at"]
    assert after_login["duration_seconds"] == duration
    assert after_login["status"] == "closed"
    assert after_login["received_at"] == received
    assert _get(client, session_b)["status"] == "active"
    assert _get(client, session_b)["logon_at"].startswith("2026-08-16T12:34:19")
    assert _active_count(client, agent_id) == 1

    _post_session(
        client,
        agent_id,
        session_a,
        logon_at=login_a,
        logoff_at=stale_close,
        status="closed",
        collected_at=stale_close,
    )
    _post_session(client, agent_id, session_a, logon_at=login_a, logoff_at=None, status="active")
    _heartbeat(client, agent_id, datetime.now(timezone.utc))
    client.get(FLEET_URL)
    client.get(f"{FLEET_URL}/{agent_id}")
    client.get(f"{FLEET_URL}/{agent_id}/sessions")

    final_a = _get(client, session_a)
    final_b = _get(client, session_b)
    assert final_a["status"] == "closed"
    assert final_a["logoff_at"].startswith("2026-08-16T12:25:25")
    assert final_a["duration_seconds"] == duration
    assert final_a["received_at"] == received
    assert final_a["logon_at"].startswith("2026-08-16T12:22:56")
    assert final_b["status"] == "active"
    assert final_b["logon_at"].startswith("2026-08-16T12:34:19")
    assert final_b["logoff_at"] is None
    listed = client.get(SESSIONS_URL, params={"agent_id": agent_id, "sort": "logon_at"}).json()
    assert listed["total"] == 2
    by_id = {row["session_id"]: row for row in listed["items"]}
    assert by_id[session_a]["logoff_at"].startswith("2026-08-16T12:25:25")
    assert by_id[session_b]["session_id"] != session_a
    assert _active_count(client, agent_id) == 1

    stale = datetime.now(timezone.utc) - timedelta(seconds=STALE_MAX_SECONDS + 30)
    _heartbeat(client, agent_id, stale)
    client.get(f"{FLEET_URL}/{agent_id}/sessions")
    after_offline = _get(client, session_a)
    assert after_offline["logoff_at"].startswith("2026-08-16T12:25:25")
    assert after_offline["duration_seconds"] == duration
    assert after_offline["status"] == "closed"
    assert after_offline["received_at"] == received


def test_repository_refuses_to_overwrite_closed_session() -> None:
    from app.domain.sessions import ComputerSession
    from app.infrastructure.memory import MemoryAgentRepository, MemorySessionRepository

    agents = MemoryAgentRepository()
    repo = MemorySessionRepository(agents)
    agent_id = uuid4()
    session_id = uuid4()
    login_a = datetime(2026, 8, 16, 12, 22, 56, tzinfo=timezone.utc)
    shutdown_at = datetime(2026, 8, 16, 12, 25, 25, tzinfo=timezone.utc)
    later = datetime(2026, 8, 16, 12, 35, 34, tzinfo=timezone.utc)
    closed = ComputerSession(
        session_id=session_id,
        agent_id=agent_id,
        username="ROHIT",
        account_sid="S-1-5-21-1",
        logon_at=login_a,
        logoff_at=shutdown_at,
        status=SESSION_STATUS_CLOSED,
        collected_at=shutdown_at,
        received_at=shutdown_at,
    )
    assert repo.upsert_many([closed]) == 1
    rewrite = ComputerSession(
        session_id=session_id,
        agent_id=agent_id,
        username="ROHIT",
        account_sid="S-1-5-21-1",
        logon_at=login_a,
        logoff_at=later,
        status=SESSION_STATUS_CLOSED,
        collected_at=later,
        received_at=later,
    )
    assert repo.upsert_many([rewrite]) == 0
    stored = repo.get(session_id)
    assert stored is not None
    assert stored.logoff_at == shutdown_at
    assert stored.status == SESSION_STATUS_CLOSED
    assert repo.close_active_for_agent(agent_id, logoff_at=later, received_at=later) == 0
    stored = repo.get(session_id)
    assert stored.logoff_at == shutdown_at


def test_shutdown_closes_only_current_session(client: TestClient) -> None:
    """Session 2 1:03, Session 3 1:15, shutdown 1:18 must not stamp Session 2 with 1:18."""
    login_2 = datetime(2026, 8, 16, 15, 3, 49, tzinfo=timezone.utc)
    login_3 = datetime(2026, 8, 16, 15, 15, 26, tzinfo=timezone.utc)
    shutdown_at = datetime(2026, 8, 16, 15, 18, 18, tzinfo=timezone.utc)
    agent_id = _register(client)
    session_2 = str(uuid4())
    session_3 = str(uuid4())

    _post_session(client, agent_id, session_2, logon_at=login_2, logoff_at=None, status="active")
    _post_session(client, agent_id, session_3, logon_at=login_3, logoff_at=None, status="active")
    previous = _get(client, session_2)
    assert previous["status"] == "closed"
    assert previous["logoff_at"].startswith("2026-08-16T15:15:26")
    assert _active_count(client, agent_id) == 1

    response = client.post(
        SESSIONS_URL,
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "collected_at": shutdown_at.isoformat(),
            "sessions": [
                {
                    "session_id": session_2,
                    "username": "ROHIT",
                    "account_sid": "S-1-5-21-1",
                    "logon_at": login_2.isoformat(),
                    "logoff_at": shutdown_at.isoformat(),
                    "status": "closed",
                },
                {
                    "session_id": session_3,
                    "username": "ROHIT",
                    "account_sid": "S-1-5-21-1",
                    "logon_at": login_3.isoformat(),
                    "logoff_at": shutdown_at.isoformat(),
                    "status": "closed",
                },
            ],
        },
    )
    assert response.status_code == 200, response.text
    after_2 = _get(client, session_2)
    after_3 = _get(client, session_3)
    assert after_2["logoff_at"].startswith("2026-08-16T15:15:26")
    assert after_2["received_at"] == previous["received_at"]
    assert after_3["status"] == "closed"
    assert after_3["logoff_at"].startswith("2026-08-16T15:18:18")
    assert _active_count(client, agent_id) == 0


def test_shutdown_does_not_close_stale_active_at_shutdown_time(client: TestClient) -> None:
    """If Session 3 was never opened on the backend, shutdown must not close Session 2 at 1:18."""
    login_2 = datetime(2026, 8, 16, 15, 3, 49, tzinfo=timezone.utc)
    login_3 = datetime(2026, 8, 16, 15, 15, 26, tzinfo=timezone.utc)
    shutdown_at = datetime(2026, 8, 16, 15, 18, 18, tzinfo=timezone.utc)
    agent_id = _register(client)
    session_2 = str(uuid4())
    session_3 = str(uuid4())

    _post_session(client, agent_id, session_2, logon_at=login_2, logoff_at=None, status="active")
    assert _active_count(client, agent_id) == 1

    _post_session(
        client,
        agent_id,
        session_3,
        logon_at=login_3,
        logoff_at=shutdown_at,
        status="closed",
        collected_at=shutdown_at,
    )
    after_2 = _get(client, session_2)
    after_3 = _get(client, session_3)
    assert after_3["status"] == "closed"
    assert after_3["logoff_at"].startswith("2026-08-16T15:18:18")
    assert after_2["status"] == "active"
    assert after_2["logoff_at"] is None

    stale = shutdown_at - timedelta(seconds=STALE_MAX_SECONDS + 30)
    _heartbeat(client, agent_id, stale)
    client.get(f"{FLEET_URL}/{agent_id}/sessions")
    reconciled = _get(client, session_2)
    assert reconciled["status"] == "active"
    assert reconciled["logoff_at"] is None
    after_3_again = _get(client, session_3)
    assert after_3_again["logoff_at"].startswith("2026-08-16T15:18:18")


def test_close_active_for_agent_fails_when_two_sessions_are_active() -> None:
    from app.infrastructure.memory import MemoryAgentRepository, MemorySessionRepository

    agents = MemoryAgentRepository()
    repo = MemorySessionRepository(agents)
    agent_id = uuid4()
    login_2 = datetime(2026, 8, 16, 15, 3, 49, tzinfo=timezone.utc)
    login_3 = datetime(2026, 8, 16, 15, 15, 26, tzinfo=timezone.utc)
    shutdown_at = datetime(2026, 8, 16, 15, 18, 18, tzinfo=timezone.utc)
    session_2 = ComputerSession(
        session_id=uuid4(),
        agent_id=agent_id,
        username="ROHIT",
        account_sid="S-1-5-21-1",
        logon_at=login_2,
        logoff_at=None,
        status=SESSION_STATUS_ACTIVE,
        collected_at=login_2,
        received_at=login_2,
    )
    session_3 = ComputerSession(
        session_id=uuid4(),
        agent_id=agent_id,
        username="ROHIT",
        account_sid="S-1-5-21-1",
        logon_at=login_3,
        logoff_at=None,
        status=SESSION_STATUS_ACTIVE,
        collected_at=login_3,
        received_at=login_3,
    )
    assert repo.upsert_many([session_2, session_3]) == 2
    assert repo.close_active_for_agent(agent_id, logoff_at=shutdown_at, received_at=shutdown_at) == 0
    assert repo.get(session_2.session_id).status == SESSION_STATUS_ACTIVE
    assert repo.get(session_3.session_id).status == SESSION_STATUS_ACTIVE
    assert repo.get(session_2.session_id).logoff_at is None
    assert repo.get(session_3.session_id).logoff_at is None


def test_new_login_closes_previous_active_at_new_logon(client: TestClient) -> None:
    """POST Session 3 active while Session 2 is still ACTIVE must close Session 2 at Session 3 login."""
    login_2 = datetime(2026, 8, 16, 15, 15, 26, tzinfo=timezone.utc)
    login_3 = datetime(2026, 8, 16, 15, 45, 59, tzinfo=timezone.utc)
    agent_id = _register(client)
    session_2 = str(uuid4())
    session_3 = str(uuid4())

    _post_session(client, agent_id, session_2, logon_at=login_2, logoff_at=None, status="active")
    assert _active_count(client, agent_id) == 1

    _post_session(client, agent_id, session_3, logon_at=login_3, logoff_at=None, status="active")
    previous = _get(client, session_2)
    current = _get(client, session_3)
    listed = client.get(SESSIONS_URL, params={"agent_id": agent_id}).json()
    actives = [item for item in listed["items"] if item["status"] == "active"]

    assert previous["status"] == "closed"
    assert previous["logoff_at"].startswith("2026-08-16T15:45:59")
    assert previous["duration_seconds"] == int((login_3 - login_2).total_seconds())
    assert current["status"] == "active"
    assert current["logoff_at"] is None
    assert current["logon_at"].startswith("2026-08-16T15:45:59")
    assert len(actives) == 1
    assert actives[0]["session_id"] == session_3
    assert _active_count(client, agent_id) == 1


def test_activate_current_closes_other_active_in_one_write() -> None:
    from app.infrastructure.memory import MemoryAgentRepository, MemorySessionRepository

    agents = MemoryAgentRepository()
    repo = MemorySessionRepository(agents)
    agent_id = uuid4()
    login_2 = datetime(2026, 8, 16, 15, 15, 26, tzinfo=timezone.utc)
    login_3 = datetime(2026, 8, 16, 15, 45, 59, tzinfo=timezone.utc)
    session_2 = ComputerSession(
        session_id=uuid4(),
        agent_id=agent_id,
        username="ROHIT",
        account_sid="S-1-5-21-1",
        logon_at=login_2,
        logoff_at=None,
        status=SESSION_STATUS_ACTIVE,
        collected_at=login_2,
        received_at=login_2,
    )
    session_3 = ComputerSession(
        session_id=uuid4(),
        agent_id=agent_id,
        username="ROHIT",
        account_sid="S-1-5-21-1",
        logon_at=login_3,
        logoff_at=None,
        status=SESSION_STATUS_ACTIVE,
        collected_at=login_3,
        received_at=login_3,
    )
    assert repo.upsert_many([session_2]) == 1
    written = repo.activate_current(session_3)
    assert written >= 2
    previous = repo.get(session_2.session_id)
    current = repo.get(session_3.session_id)
    assert previous.status == SESSION_STATUS_CLOSED
    assert previous.logoff_at == login_3
    assert current.status == SESSION_STATUS_ACTIVE
    assert current.logoff_at is None
    actives = repo.list(
        ListSessionsQuery(agent_id=agent_id, status=SESSION_STATUS_ACTIVE, limit=200, sort="logon_at")
    ).items
    assert len(actives) == 1
    assert actives[0].session_id == session_3.session_id


