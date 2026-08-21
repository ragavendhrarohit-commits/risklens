from __future__ import annotations

from pathlib import Path

from app.domain.classification import (
    BusinessCategory,
    ClassificationEngine,
    ClassificationInput,
    PlatformSeedMappingProvider,
    ProductivityClass,
    RiskLevel,
    default_classification_engine,
)

ENGINE_SOURCE = (
    Path(__file__).resolve().parents[1] / "app" / "domain" / "classification" / "engine.py"
)


class _StubProvider:
    """Prove the engine consults the provider, not a hardcoded catalog."""

    def is_browser_container(self, executable_key: str) -> bool:
        return executable_key == "cont.exe"

    def category_for_executable(self, executable_key: str) -> BusinessCategory | None:
        if executable_key == "tool.exe":
            return BusinessCategory.ERP
        return None

    def category_for_host(self, host: str) -> BusinessCategory | None:
        if host == "example.test":
            return BusinessCategory.FINANCE
        return None

    def productivity_for_category(self, category: BusinessCategory) -> ProductivityClass:
        if category is BusinessCategory.ERP:
            return ProductivityClass.PRODUCTIVE
        if category is BusinessCategory.FINANCE:
            return ProductivityClass.UNPRODUCTIVE
        if category is BusinessCategory.OTHER:
            return ProductivityClass.NEUTRAL
        if category is BusinessCategory.BROWSER:
            return ProductivityClass.NEUTRAL
        return ProductivityClass.NEUTRAL

    def risk_for_category(self, category: BusinessCategory) -> RiskLevel:
        if category is BusinessCategory.ERP:
            return RiskLevel.LOW
        if category is BusinessCategory.FINANCE:
            return RiskLevel.HIGH
        return RiskLevel.MEDIUM

    def productivity_for_host(self, host: str) -> ProductivityClass | None:
        return None

    def risk_for_host(self, host: str) -> RiskLevel | None:
        return None


def _classify(executable: str, domain: str | None = None):
    return default_classification_engine().classify(
        ClassificationInput(executable=executable, browser_domain=domain)
    )


def test_desktop_application_classification() -> None:
    code = _classify("Code.exe")
    assert code.category is BusinessCategory.DEVELOPMENT
    assert code.productivity is ProductivityClass.PRODUCTIVE
    assert code.risk is RiskLevel.LOW
    assert code.rule == "executable_catalog"

    excel = _classify("EXCEL.EXE")
    assert excel.category is BusinessCategory.OFFICE
    assert excel.productivity is ProductivityClass.PRODUCTIVE

    teams = _classify("ms-teams.exe")
    assert teams.category is BusinessCategory.COMMUNICATION
    assert teams.productivity is ProductivityClass.PRODUCTIVE

    explorer = _classify("Explorer.exe")
    assert explorer.category is BusinessCategory.SYSTEM
    assert explorer.productivity is ProductivityClass.IGNORED
    assert explorer.risk is RiskLevel.LOW


def test_browser_domain_overrides_browser_executable() -> None:
    result = _classify("chrome.exe", "github.com")
    assert result.category is BusinessCategory.DEVELOPMENT
    assert result.category is not BusinessCategory.BROWSER
    assert result.productivity is ProductivityClass.PRODUCTIVE
    assert result.risk is RiskLevel.LOW
    assert result.browser_host == "github.com"
    assert result.rule == "host_catalog"

    youtube = _classify("msedge.exe", "https://www.youtube.com/watch?v=1")
    assert youtube.category is BusinessCategory.ENTERTAINMENT
    assert youtube.productivity is ProductivityClass.UNPRODUCTIVE
    assert youtube.browser_host == "youtube.com"

    no_host = _classify("firefox.exe")
    assert no_host.category is BusinessCategory.BROWSER
    assert no_host.productivity is ProductivityClass.NEUTRAL
    assert no_host.risk is RiskLevel.MEDIUM


def test_unknown_executable() -> None:
    result = _classify("AcmeTool.exe")
    assert result.category is BusinessCategory.OTHER
    assert result.productivity is ProductivityClass.NEUTRAL
    assert result.risk is RiskLevel.MEDIUM
    assert result.rule == "unknown_executable"


def test_unknown_browser_domain() -> None:
    result = _classify("chrome.exe", "new-saas.example")
    assert result.productivity is ProductivityClass.NEUTRAL
    assert result.category is BusinessCategory.OTHER
    assert result.risk is RiskLevel.MEDIUM
    assert result.rule == "unknown_domain"


def test_category_productivity_and_risk_returned() -> None:
    salesforce = _classify("chrome.exe", "salesforce.com")
    assert salesforce.category is BusinessCategory.CRM
    assert salesforce.productivity is ProductivityClass.PRODUCTIVE
    assert salesforce.risk is RiskLevel.LOW

    chatgpt = _classify("chrome.exe", "chat.openai.com")
    assert chatgpt.category is BusinessCategory.AI_ASSISTANT
    assert chatgpt.productivity is ProductivityClass.PRODUCTIVE
    assert chatgpt.risk is RiskLevel.MEDIUM

    mega = _classify("chrome.exe", "mega.nz")
    assert mega.category is BusinessCategory.FILE_SHARING
    assert mega.productivity is ProductivityClass.NEUTRAL
    assert mega.risk is RiskLevel.HIGH


def test_non_browser_ignores_domain() -> None:
    result = _classify("Cursor.exe", "github.com")
    assert result.category is BusinessCategory.DEVELOPMENT
    assert result.browser_host is None
    assert result.rule == "executable_catalog"


def test_engine_uses_injected_mapping_provider() -> None:
    engine = ClassificationEngine(_StubProvider())
    desktop = engine.classify(ClassificationInput("tool.exe"))
    assert desktop.category is BusinessCategory.ERP
    assert desktop.productivity is ProductivityClass.PRODUCTIVE
    assert desktop.risk is RiskLevel.LOW

    browser = engine.classify(ClassificationInput("cont.exe", "example.test"))
    assert browser.category is BusinessCategory.FINANCE
    assert browser.productivity is ProductivityClass.UNPRODUCTIVE
    assert browser.risk is RiskLevel.HIGH


def test_engine_contains_no_catalog_literals() -> None:
    source = ENGINE_SOURCE.read_text(encoding="utf-8")
    for token in (
        "github.com",
        "chrome.exe",
        "code.exe",
        "youtube.com",
        "salesforce.com",
        "explorer.exe",
    ):
        assert token not in source
    assert "PlatformSeedMappingProvider" in (
        Path(__file__).resolve().parents[1]
        / "app"
        / "domain"
        / "classification"
        / "platform_seed.py"
    ).read_text(encoding="utf-8")
    assert isinstance(PlatformSeedMappingProvider(), object)
