from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from typing import Iterable

from app.domain.website_usage import (
    UNKNOWN_BROWSER_WEBSITE,
    browser_label,
    format_total_time,
    parse_website_from_title,
)


@dataclass(frozen=True, slots=True)
class ApplicationAggregate:
    application: str
    duration_seconds: int
    launch_count: int


@dataclass(frozen=True, slots=True)
class TitleAggregate:
    window_title: str
    executable_name: str
    duration_seconds: int
    visit_count: int


@dataclass(frozen=True, slots=True)
class IntervalPoint:
    started_at: datetime
    ended_at: datetime


@dataclass(frozen=True, slots=True)
class SpanMetrics:
    first_activity: datetime | None
    last_activity: datetime | None
    working_time_seconds: int
    active_time_seconds: int
    segment_count: int
    applications_used: int
    longest_continuous_working_session_seconds: int
    longest_idle_period_seconds: int


def _ensure_aware(value: datetime) -> datetime:
    if value.tzinfo is None:
        return value.replace(tzinfo=timezone.utc)
    return value


def merge_span_metrics(
    *,
    first_activity: datetime | None,
    last_activity: datetime | None,
    active_time_seconds: int,
    segment_count: int,
    applications_used: int,
    intervals: Iterable[IntervalPoint],
) -> SpanMetrics:
    points = sorted(
        (
            IntervalPoint(_ensure_aware(row.started_at), _ensure_aware(row.ended_at))
            for row in intervals
            if row.started_at is not None and row.ended_at is not None
        ),
        key=lambda row: (row.started_at, row.ended_at),
    )
    longest_session = 0
    longest_idle = 0
    if points:
        merge_start = points[0].started_at
        merge_end = points[0].ended_at
        for point in points[1:]:
            if point.started_at <= merge_end:
                if point.ended_at > merge_end:
                    merge_end = point.ended_at
                continue
            longest_session = max(longest_session, int((merge_end - merge_start).total_seconds()))
            gap = int((point.started_at - merge_end).total_seconds())
            if gap > longest_idle:
                longest_idle = gap
            merge_start = point.started_at
            merge_end = point.ended_at
        longest_session = max(longest_session, int((merge_end - merge_start).total_seconds()))

    working = 0
    if first_activity is not None and last_activity is not None:
        working = max(0, int((_ensure_aware(last_activity) - _ensure_aware(first_activity)).total_seconds()))

    return SpanMetrics(
        first_activity=first_activity,
        last_activity=last_activity,
        working_time_seconds=working,
        active_time_seconds=max(0, int(active_time_seconds or 0)),
        segment_count=max(0, int(segment_count or 0)),
        applications_used=max(0, int(applications_used or 0)),
        longest_continuous_working_session_seconds=max(0, longest_session),
        longest_idle_period_seconds=max(0, longest_idle),
    )


def rollup_websites(title_rows: Iterable[TitleAggregate]) -> list[tuple[str, int, int]]:
    """Return (website, duration_seconds, visits) sorted by duration desc."""
    buckets: dict[str, list[int]] = {}
    for row in title_rows:
        if browser_label(row.executable_name) not in {"Chrome", "Edge", "Firefox"}:
            continue
        website = parse_website_from_title(row.window_title or "")
        if not website:
            website = UNKNOWN_BROWSER_WEBSITE
        current = buckets.get(website)
        if current is None:
            buckets[website] = [max(0, int(row.duration_seconds or 0)), max(0, int(row.visit_count or 0))]
        else:
            current[0] += max(0, int(row.duration_seconds or 0))
            current[1] += max(0, int(row.visit_count or 0))
    rows = [(website, data[0], data[1]) for website, data in buckets.items()]
    rows.sort(key=lambda item: (-item[1], item[0]))
    return rows


def application_usage_rows(
    aggregates: Iterable[ApplicationAggregate],
    *,
    active_time_seconds: int,
) -> list[tuple[str, int, float, int, str]]:
    active = max(0, int(active_time_seconds or 0))
    rows = []
    for item in aggregates:
        duration = max(0, int(item.duration_seconds or 0))
        percentage = round((duration / active) * 100, 1) if active else 0.0
        rows.append(
            (
                (item.application or "").strip() or "Unknown Application",
                duration,
                percentage,
                max(0, int(item.launch_count or 0)),
                format_total_time(duration),
            )
        )
    rows.sort(key=lambda item: (-item[1], item[0]))
    return rows


def idle_time_seconds(working_time_seconds: int, active_time_seconds: int) -> int:
    return max(0, int(working_time_seconds or 0) - int(active_time_seconds or 0))


def average_session_length_seconds(active_time_seconds: int, segment_count: int) -> int:
    if segment_count <= 0:
        return 0
    return max(0, int(active_time_seconds or 0) // int(segment_count))


def interval_end(started_at: datetime, stopped_at: datetime | None, duration_seconds: int) -> datetime:
    if stopped_at is not None:
        return _ensure_aware(stopped_at)
    start = _ensure_aware(started_at)
    return start + timedelta(seconds=max(0, int(duration_seconds or 0)))
