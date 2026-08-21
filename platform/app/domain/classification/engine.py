from __future__ import annotations

from pathlib import PureWindowsPath
from urllib.parse import urlparse

from app.domain.classification.entity import Classification, ClassificationInput
from app.domain.classification.provider import MappingProvider
from app.domain.classification.types import BusinessCategory

RULE_HOST = "host_catalog"
RULE_BROWSER_NO_HOST = "browser_without_host"
RULE_EXECUTABLE = "executable_catalog"
RULE_UNKNOWN_HOST = "unknown_domain"
RULE_UNKNOWN_EXECUTABLE = "unknown_executable"


class ClassificationEngine:
    """Resolves category, productivity, and risk. Mappings come from the provider."""

    def __init__(self, mappings: MappingProvider) -> None:
        self._mappings = mappings

    def classify(self, activity: ClassificationInput) -> Classification:
        executable_key = normalize_executable(activity.executable)
        host = normalize_host(activity.browser_domain)

        if self._mappings.is_browser_container(executable_key):
            return self._classify_browser(executable_key, host)
        return self._classify_desktop(executable_key)

    def _classify_browser(self, executable_key: str, host: str | None) -> Classification:
        if not host:
            return self._finish(
                category=BusinessCategory.BROWSER,
                executable_key=executable_key,
                host=None,
                rule=RULE_BROWSER_NO_HOST,
            )
        category = self._mappings.category_for_host(host)
        if category is None:
            return self._finish(
                category=BusinessCategory.OTHER,
                executable_key=executable_key,
                host=host,
                rule=RULE_UNKNOWN_HOST,
            )
        return self._finish(
            category=category,
            executable_key=executable_key,
            host=host,
            rule=RULE_HOST,
        )

    def _classify_desktop(self, executable_key: str) -> Classification:
        category = self._mappings.category_for_executable(executable_key)
        if category is None:
            return self._finish(
                category=BusinessCategory.OTHER,
                executable_key=executable_key,
                host=None,
                rule=RULE_UNKNOWN_EXECUTABLE,
            )
        return self._finish(
            category=category,
            executable_key=executable_key,
            host=None,
            rule=RULE_EXECUTABLE,
        )

    def _finish(
        self,
        *,
        category: BusinessCategory,
        executable_key: str,
        host: str | None,
        rule: str,
    ) -> Classification:
        productivity = (
            self._mappings.productivity_for_host(host)
            if host
            else None
        ) or self._mappings.productivity_for_category(category)
        risk = (
            self._mappings.risk_for_host(host) if host else None
        ) or self._mappings.risk_for_category(category)
        return Classification(
            category=category,
            productivity=productivity,
            risk=risk,
            executable_key=executable_key,
            browser_host=host,
            rule=rule,
        )


def default_classification_engine() -> ClassificationEngine:
    from app.domain.classification.platform_seed import PlatformSeedMappingProvider

    return ClassificationEngine(PlatformSeedMappingProvider())


def normalize_executable(value: str) -> str:
    raw = (value or "").strip()
    if not raw:
        return ""
    name = PureWindowsPath(raw.replace("/", "\\")).name
    return name.casefold()


def normalize_host(value: str | None) -> str | None:
    if value is None:
        return None
    raw = value.strip()
    if not raw:
        return None
    candidate = raw if "://" in raw else f"https://{raw}"
    host = urlparse(candidate).hostname
    if not host:
        host = raw.split("/")[0].split(":")[0]
    host = host.strip(".").casefold()
    if host.startswith("www."):
        host = host[4:]
    return host or None
