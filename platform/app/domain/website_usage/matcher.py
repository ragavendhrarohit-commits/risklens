from __future__ import annotations

from dataclasses import dataclass

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
