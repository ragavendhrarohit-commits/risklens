from __future__ import annotations

from urllib.parse import urlparse

from app.domain.classification.engine import normalize_host

_FORBIDDEN_SCHEMES = frozenset(
    {
        "chrome",
        "chrome-extension",
        "edge",
        "extension",
        "about",
        "file",
        "data",
        "blob",
        "javascript",
        "view-source",
        "devtools",
        "moz-extension",
    }
)


def strip_url(raw: str) -> tuple[str, str]:
    """Return (host, origin-only URL). Drops userinfo, path, query, fragment."""
    text = (raw or "").strip()
    if not text:
        raise ValueError("url is required")
    candidate = text if "://" in text else f"https://{text}"
    parsed = urlparse(candidate)
    scheme = (parsed.scheme or "").lower()
    if scheme in _FORBIDDEN_SCHEMES or scheme not in {"http", "https"}:
        raise ValueError("forbidden url scheme")
    host = normalize_host(parsed.hostname or text)
    if not host:
        raise ValueError("host is required")
    port = parsed.port
    if (scheme == "https" and port == 443) or (scheme == "http" and port == 80):
        port = None
    origin = f"{scheme}://{host}"
    if port:
        origin = f"{origin}:{port}"
    return host, origin
