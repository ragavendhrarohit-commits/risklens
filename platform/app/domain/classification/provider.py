from __future__ import annotations

from typing import Protocol

from app.domain.classification.types import BusinessCategory, ProductivityClass, RiskLevel


class MappingProvider(Protocol):
    """Catalog and policy lookups. The engine does not embed product mappings."""

    def is_browser_container(self, executable_key: str) -> bool:
        """True when the executable is a web browser, not a business category."""

    def category_for_executable(self, executable_key: str) -> BusinessCategory | None:
        """Desktop mapping. None means unknown executable."""

    def category_for_host(self, host: str) -> BusinessCategory | None:
        """Domain mapping. None means unknown host."""

    def productivity_for_category(self, category: BusinessCategory) -> ProductivityClass:
        """Default productivity for a category. Company remaps belong here later."""

    def risk_for_category(self, category: BusinessCategory) -> RiskLevel:
        """Default risk for a category."""

    def productivity_for_host(self, host: str) -> ProductivityClass | None:
        """Optional asset productivity override."""

    def risk_for_host(self, host: str) -> RiskLevel | None:
        """Optional asset risk override (e.g. Mega High)."""
