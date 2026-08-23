from __future__ import annotations

import re
from dataclasses import dataclass
from datetime import datetime
from uuid import UUID

# Longer keywords first so "GeeksforGeeks" wins over a shorter overlap.
_WEBSITE_RULES: tuple[tuple[str, str, str], ...] = (
    ("geeksforgeeks", "GeeksforGeeks", "Learning"),
    ("confluence", "Confluence", "Project Management"),
    ("linkedin", "LinkedIn", "Social Media"),
    ("youtube", "YouTube", "Entertainment"),
    ("chatgpt", "ChatGPT", "AI Assistant"),
    ("proxmox", "Proxmox", "Infrastructure"),
    ("github", "GitHub", "Development"),
    ("office", "Microsoft Office", "Office"),
    ("jira", "Jira", "Project Management"),
)

_BROWSER_LABELS: dict[str, str] = {
    "chrome.exe": "Chrome",
    "msedge.exe": "Edge",
    "firefox.exe": "Firefox",
    "brave.exe": "Brave",
    "opera.exe": "Opera",
    "vivaldi.exe": "Vivaldi",
    "iexplore.exe": "Internet Explorer",
}

UNKNOWN_WEBSITE = "Other"
UNKNOWN_CATEGORY = "Other"
UNKNOWN_BROWSER_WEBSITE = "Unknown Website"

# Exact last/first Chrome title segments, plus substring fallbacks.
# Matching runs after the browser suffix is stripped so "Google Chrome"
# never becomes google.com.
_SITE_LABELS: dict[str, str] = {
    "github": "github.com",
    "youtube": "youtube.com",
    "gmail": "gmail.com",
    "linkedin": "linkedin.com",
    "chatgpt": "chat.openai.com",
    "openai": "chat.openai.com",
    "claude": "claude.ai",
    "anthropic": "claude.ai",
    "udemy": "udemy.com",
    "google search": "google.com",
    "google docs": "docs.google.com",
    "google drive": "drive.google.com",
    "google calendar": "calendar.google.com",
    "google": "google.com",
    "stack overflow": "stackoverflow.com",
    "stackoverflow": "stackoverflow.com",
    "reddit": "reddit.com",
    "twitter": "twitter.com",
    "facebook": "facebook.com",
    "instagram": "instagram.com",
    "slack": "slack.com",
    "discord": "discord.com",
    "notion": "notion.so",
    "figma": "figma.com",
    "gitlab": "gitlab.com",
    "bitbucket": "bitbucket.org",
    "jira": "jira.atlassian.com",
    "confluence": "confluence.atlassian.com",
    "office": "office.com",
    "microsoft office": "office.com",
    "outlook": "outlook.office.com",
    "teams": "teams.microsoft.com",
    "proxmox": "proxmox.com",
    "risklens": "risklens.local",
    "geeksforgeeks": "geeksforgeeks.org",
    "whatsapp": "web.whatsapp.com",
    "wikipedia": "wikipedia.org",
}
_WEBSITE_DOMAINS: tuple[tuple[str, str], ...] = tuple(
    (label, domain) for label, domain in sorted(_SITE_LABELS.items(), key=lambda item: -len(item[0]))
)

_FILE_LIKE_TLDS = frozenset(
    {
        "py",
        "js",
        "ts",
        "tsx",
        "jsx",
        "go",
        "rs",
        "md",
        "html",
        "htm",
        "css",
        "json",
        "xml",
        "yml",
        "yaml",
        "txt",
        "pdf",
        "png",
        "jpg",
        "exe",
        "dll",
        "zip",
    }
)
_GENERIC_BROWSER_TITLES = frozenset(
    {
        "",
        "new tab",
        "untitled",
        "untitled tab",
        "settings",
        "extensions",
        "downloads",
        "history",
        "bookmarks",
        "task manager",
    }
)

_HOSTNAME_RE = re.compile(
    r"\b(?:www\.)?([a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z]{2,})+)\b",
    re.IGNORECASE,
)
_BROWSER_TITLE_SUFFIX = re.compile(
    r"\s*[-–—]\s*(?:Google Chrome|Microsoft Edge|Mozilla Firefox|Brave|Opera|"
    r"Vivaldi|Internet Explorer)\s*$",
    re.IGNORECASE,
)
_BROWSER_ONLY_TITLE = re.compile(
    r"^(?:Google Chrome|Microsoft Edge|Mozilla Firefox|Brave|Opera|"
    r"Vivaldi|Internet Explorer)$",
    re.IGNORECASE,
)
_GITHUB_TITLE_RE = re.compile(
    r"(?:^github\b|\bgithub$|\bgithub\s*[-–—]|[·|]\s*github\b)",
    re.IGNORECASE,
)
_GITHUB_REPO_RE = re.compile(r"^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+(?:[:\s].*)?$")
_TITLE_SPLIT = re.compile(r"\s+[-–—]\s+|\s*[|·]\s*")
_PAGE_SITE_SUFFIX = {
    "youtube.com": re.compile(r"\s*[-–—]\s*YouTube\s*$", re.IGNORECASE),
    "google.com": re.compile(r"\s*[-–—]\s*(?:Google Search|Google)\s*$", re.IGNORECASE),
    "gmail.com": re.compile(r"\s*[-–—]\s*Gmail\s*$", re.IGNORECASE),
    "linkedin.com": re.compile(r"\s*[|·-–—]\s*LinkedIn\s*$", re.IGNORECASE),
    "chat.openai.com": re.compile(r"\s*[-–—]\s*ChatGPT\s*$", re.IGNORECASE),
}


@dataclass(frozen=True, slots=True)
class WebsiteMatch:
    website: str
    category: str


@dataclass(frozen=True, slots=True)
class WebsiteUsageRow:
    browser: str
    website: str
    duration_seconds: int
    category: str
    total_time: str


@dataclass(frozen=True, slots=True)
class BrowserActivitySegment:
    id: str
    session_id: UUID
    browser: str
    website: str
    page_title: str
    window_title: str
    started_at: datetime
    stopped_at: datetime | None
    duration_seconds: int
    status: str
    username: str
    machine_name: str


@dataclass(frozen=True, slots=True)
class BrowserActivityPage:
    items: list[BrowserActivitySegment]
    total: int
    limit: int
    offset: int
    sort: str


@dataclass(frozen=True, slots=True)
class BrowserVisit:
    started_at: datetime
    stopped_at: datetime | None
    duration_seconds: int
    status: str
    page_title: str
    window_title: str
    website: str
    browser: str


@dataclass(frozen=True, slots=True)
class BrowserActivitySummaryRow:
    website: str
    browser: str
    page_title: str
    duration_seconds: int
    total_time: str
    visits: int
    last_visited: datetime
    status: str
    timeline: list[BrowserVisit]


@dataclass(frozen=True, slots=True)
class BrowserActivitySummary:
    items: list[BrowserActivitySummaryRow]
    total: int
    limit: int
    offset: int
    sort: str
    total_browser_time_seconds: int
    total_browser_time: str
    websites_visited: int
    active_website: str | None
    active_browser: str | None


def normalize_executable(value: str) -> str:
    raw = (value or "").strip().replace("/", "\\")
    name = raw.rsplit("\\", 1)[-1]
    return name.casefold()


def browser_label(executable: str) -> str | None:
    return _BROWSER_LABELS.get(normalize_executable(executable))


def match_website(window_title: str) -> WebsiteMatch:
    text = (window_title or "").casefold()
    for keyword, website, category in _WEBSITE_RULES:
        if keyword in text:
            return WebsiteMatch(website=website, category=category)
    return WebsiteMatch(website=UNKNOWN_WEBSITE, category=UNKNOWN_CATEGORY)


def strip_browser_suffix(window_title: str) -> str:
    title = _BROWSER_TITLE_SUFFIX.sub("", window_title or "").strip()
    if _BROWSER_ONLY_TITLE.fullmatch(title):
        return ""
    return title


def parse_website_from_title(window_title: str) -> str:
    """Best-effort website from a browser window title. Never a full URL."""
    return parse_browser_window_title(window_title)[0]


def parse_page_title(window_title: str, website: str | None = None) -> str:
    """Window title for display. Keep site names; only drop the browser suffix."""
    del website
    title = strip_browser_suffix(window_title)
    return title or (window_title or "").strip()


def parse_browser_window_title(window_title: str) -> tuple[str, str]:
    """Return (website, window_title) from an Application Activity window title."""
    title = strip_browser_suffix(window_title or "")
    if not title:
        return UNKNOWN_BROWSER_WEBSITE, ""
    website = _infer_website(title)
    return website, parse_page_title(window_title, website)


def _title_parts(title: str) -> list[str]:
    return [part.strip() for part in _TITLE_SPLIT.split(title or "") if part.strip()]


def _domain_from_label(label: str) -> str | None:
    folded = (label or "").strip().casefold()
    if not folded:
        return None
    if folded in _SITE_LABELS:
        return _SITE_LABELS[folded]
    if _GITHUB_REPO_RE.match((label or "").strip()):
        return "github.com"
    return _hostname_from_title(label)


def _hostname_from_title(title: str) -> str | None:
    found = _HOSTNAME_RE.search(title or "")
    if not found:
        return None
    host = found.group(1).casefold()
    tld = host.rsplit(".", 1)[-1]
    if tld in _FILE_LIKE_TLDS:
        return None
    return host


def _stored_domain(record: object) -> str | None:
    """Use a domain already stored on the record. Never invent one here."""
    raw = getattr(record, "domain", None)
    if raw is None or raw == "":
        raw = getattr(record, "website", None)
    text = str(raw or "").strip()
    if not text:
        return None
    folded = text.casefold()
    if folded in {UNKNOWN_BROWSER_WEBSITE.casefold(), "unknown", "other", "n/a", "-"}:
        return None
    if "." in text:
        host = folded[4:] if folded.startswith("www.") else folded
        tld = host.rsplit(".", 1)[-1]
        if tld in _FILE_LIKE_TLDS:
            return None
        return host
    return _SITE_LABELS.get(folded)


def _label_in_title(folded: str) -> str | None:
    for label, domain in _WEBSITE_DOMAINS:
        if label == "google":
            continue
        if re.search(rf"(?<![a-z0-9]){re.escape(label)}(?![a-z0-9])", folded):
            return domain
    if re.search(r"(?<![a-z0-9])google search(?![a-z0-9])", folded) or folded == "google":
        return "google.com"
    return None


def _infer_website(title: str) -> str:
    folded = (title or "").casefold()
    if folded in _GENERIC_BROWSER_TITLES:
        return UNKNOWN_BROWSER_WEBSITE
    parts = _title_parts(title)
    for label in ((parts[-1], parts[0]) if parts else ()):
        domain = _domain_from_label(label)
        if domain:
            return domain
    for part in parts[1:-1]:
        domain = _domain_from_label(part)
        if domain:
            return domain
    if _GITHUB_TITLE_RE.search(title or ""):
        return "github.com"
    hostname = _hostname_from_title(title)
    if hostname:
        return hostname
    labeled = _label_in_title(folded)
    if labeled:
        return labeled
    return UNKNOWN_BROWSER_WEBSITE


def format_total_time(seconds: int) -> str:
    total = max(int(seconds), 0)
    hours, remainder = divmod(total, 3600)
    minutes, secs = divmod(remainder, 60)
    if hours:
        return f"{hours}h {minutes:02d}m"
    if minutes:
        return f"{minutes}m"
    return f"{secs}s"


def summarize_website_usage(records: list) -> list[WebsiteUsageRow]:
    """Roll up browser Application Activity rows by executable + website.

    Non-browser executables are ignored. Duration comes from existing
    application activity seconds. No extra time is invented.
    """
    buckets: dict[tuple[str, str], list] = {}
    for record in records:
        browser = browser_label(getattr(record, "executable_name", "") or "")
        if browser is None:
            continue
        matched = match_website(getattr(record, "window_title", "") or "")
        key = (browser, matched.website)
        current = buckets.get(key)
        if current is None:
            buckets[key] = [matched.category, max(int(getattr(record, "duration_seconds", 0) or 0), 0)]
        else:
            current[1] += max(int(getattr(record, "duration_seconds", 0) or 0), 0)

    rows = [
        WebsiteUsageRow(
            browser=browser,
            website=website,
            duration_seconds=data[1],
            category=data[0],
            total_time=format_total_time(data[1]),
        )
        for (browser, website), data in buckets.items()
    ]
    rows.sort(key=lambda row: (-row.duration_seconds, row.browser, row.website))
    return rows


def browser_activity_segments(records: list) -> list[BrowserActivitySegment]:
    """One Browser Activity row per Chrome/Edge/Firefox Application Activity segment."""
    rows: list[BrowserActivitySegment] = []
    for record in records:
        browser = browser_label(getattr(record, "executable_name", "") or "")
        if browser not in {"Chrome", "Edge", "Firefox"}:
            continue
        title = getattr(record, "window_title", "") or ""
        stored = _stored_domain(record)
        parsed_website, page_title = parse_browser_window_title(title)
        website = stored or parsed_website
        display_title = page_title or strip_browser_suffix(title) or title
        stopped_at = getattr(record, "stopped_at", None)
        rows.append(
            BrowserActivitySegment(
                id=str(getattr(record, "id", "") or ""),
                session_id=getattr(record, "session_id"),
                browser=browser,
                website=website,
                page_title=display_title,
                window_title=title,
                started_at=getattr(record, "started_at"),
                stopped_at=stopped_at,
                duration_seconds=max(int(getattr(record, "duration_seconds", 0) or 0), 0),
                status="active" if stopped_at is None else "inactive",
                username=getattr(record, "username", "") or "",
                machine_name=getattr(record, "machine_name", "") or "",
            )
        )
    return rows


def _visit_end(segment: BrowserActivitySegment) -> datetime:
    return segment.stopped_at or segment.started_at


def summarize_browser_activity(segments: list[BrowserActivitySegment]) -> list[BrowserActivitySummaryRow]:
    """Group existing Browser Activity segments by website and browser."""
    buckets: dict[tuple[str, str], list[BrowserActivitySegment]] = {}
    for segment in segments:
        key = (segment.website, segment.browser)
        current = buckets.get(key)
        if current is None:
            buckets[key] = [segment]
        else:
            current.append(segment)

    rows: list[BrowserActivitySummaryRow] = []
    for (website, browser), items in buckets.items():
        items.sort(key=lambda row: row.started_at, reverse=True)
        duration = sum(max(int(row.duration_seconds or 0), 0) for row in items)
        active = any(row.status == "active" for row in items)
        last_visited = max(_visit_end(row) for row in items)
        latest_title = next((row.page_title for row in items if row.page_title), "")
        rows.append(
            BrowserActivitySummaryRow(
                website=website,
                browser=browser,
                page_title=latest_title,
                duration_seconds=duration,
                total_time=format_total_time(duration),
                visits=len(items),
                last_visited=last_visited,
                status="active" if active else "inactive",
                timeline=[
                    BrowserVisit(
                        started_at=row.started_at,
                        stopped_at=row.stopped_at,
                        duration_seconds=max(int(row.duration_seconds or 0), 0),
                        status=row.status,
                        page_title=row.page_title or row.window_title,
                        window_title=row.window_title,
                        website=website,
                        browser=browser,
                    )
                    for row in items
                ],
            )
        )
    return rows


def browser_activity_overview(segments: list[BrowserActivitySegment]) -> tuple[str | None, str | None, int]:
    """Active site/browser plus unique website count from existing segments."""
    websites = {row.website for row in segments}
    active = [row for row in segments if row.status == "active"]
    if not active:
        return None, None, len(websites)
    latest = max(active, key=lambda row: row.started_at)
    return latest.website, latest.browser, len(websites)
