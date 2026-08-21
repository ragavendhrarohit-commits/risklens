from __future__ import annotations

from dataclasses import dataclass

from app.domain.classification.types import BusinessCategory, ProductivityClass, RiskLevel


@dataclass(frozen=True, slots=True)
class ClassificationInput:
    """One usage interval: desktop executable plus optional active-tab host."""

    executable: str
    browser_domain: str | None = None


@dataclass(frozen=True, slots=True)
class Classification:
    """Labels for one interval. Time is not computed here."""

    category: BusinessCategory
    productivity: ProductivityClass
    risk: RiskLevel
    executable_key: str
    browser_host: str | None
    rule: str
