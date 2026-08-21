from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timedelta
from uuid import UUID

from app.domain.browser.entity import is_browser_executable
from app.domain.classification.engine import normalize_executable, normalize_host


@dataclass(frozen=True, slots=True)
class ApplicationInterval:
    agent_id: UUID
    session_id: UUID
    executable: str
    started_at: datetime
    stopped_at: datetime | None
    duration_seconds: int


@dataclass(frozen=True, slots=True)
class BrowserLabel:
    agent_id: UUID
    session_id: UUID
    executable: str
    host: str
    started_at: datetime
    stopped_at: datetime | None


@dataclass(frozen=True, slots=True)
class LabeledSlice:
    executable: str
    host: str | None
    started_at: datetime
    stopped_at: datetime
    duration_seconds: int


def application_total_seconds(intervals: list[ApplicationInterval]) -> int:
    return sum(max(item.duration_seconds, 0) for item in intervals)


def join_application_interval(
    app: ApplicationInterval,
    labels: list[BrowserLabel],
) -> list[LabeledSlice]:
    """Attach browser hosts to an application interval without adding time.

    The clock is Application Activity `duration_seconds`. Browser labels only
    split that budget. Sum of slices always equals the application seconds.
    """
    budget = max(app.duration_seconds, 0)
    wall_start = app.started_at
    if app.stopped_at is not None and app.stopped_at > wall_start:
        wall_end = app.stopped_at
    else:
        wall_end = wall_start + timedelta(seconds=max(budget, 1))
    wall = (wall_end - wall_start).total_seconds() or 1.0

    if budget == 0:
        return [
            LabeledSlice(
                executable=app.executable,
                host=None,
                started_at=wall_start,
                stopped_at=wall_end,
                duration_seconds=0,
            )
        ]

    if not is_browser_executable(app.executable):
        return [
            LabeledSlice(
                executable=app.executable,
                host=None,
                started_at=wall_start,
                stopped_at=wall_end,
                duration_seconds=budget,
            )
        ]

    exe = normalize_executable(app.executable)
    segments: list[tuple[datetime, datetime, str]] = []
    for label in labels:
        if label.agent_id != app.agent_id or label.session_id != app.session_id:
            continue
        if normalize_executable(label.executable) != exe:
            continue
        host = normalize_host(label.host)
        if not host:
            continue
        start = max(label.started_at, wall_start)
        end = min(label.stopped_at or wall_end, wall_end)
        if end <= start:
            continue
        segments.append((start, end, host))

    cuts = {wall_start, wall_end}
    for start, end, _host in segments:
        cuts.add(start)
        cuts.add(end)
    points = sorted(cuts)
    raw: list[tuple[datetime, datetime, str | None, float]] = []
    for start, end in zip(points, points[1:]):
        host: str | None = None
        for seg_start, seg_end, seg_host in segments:
            if seg_start <= start and end <= seg_end:
                host = seg_host
                break
        raw.append((start, end, host, (end - start).total_seconds()))

    if not raw:
        return [
            LabeledSlice(
                executable=app.executable,
                host=None,
                started_at=wall_start,
                stopped_at=wall_end,
                duration_seconds=budget,
            )
        ]

    slices: list[LabeledSlice] = []
    remaining = budget
    for index, (start, end, host, wall_sec) in enumerate(raw):
        if index == len(raw) - 1:
            duration = remaining
        else:
            duration = int(round(budget * wall_sec / wall))
            duration = min(max(duration, 0), remaining)
            remaining -= duration
        slices.append(
            LabeledSlice(
                executable=app.executable,
                host=host,
                started_at=start,
                stopped_at=end,
                duration_seconds=duration,
            )
        )
    return slices
