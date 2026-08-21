from __future__ import annotations

from app.domain.classification.types import BusinessCategory, ProductivityClass, RiskLevel

_BROWSER_CONTAINERS = frozenset(
    {
        "chrome.exe",
        "msedge.exe",
        "firefox.exe",
        "brave.exe",
        "opera.exe",
        "vivaldi.exe",
        "iexplore.exe",
    }
)

_EXECUTABLE_CATEGORY: dict[str, BusinessCategory] = {
    "code.exe": BusinessCategory.DEVELOPMENT,
    "devenv.exe": BusinessCategory.DEVELOPMENT,
    "cursor.exe": BusinessCategory.DEVELOPMENT,
    "excel.exe": BusinessCategory.OFFICE,
    "winword.exe": BusinessCategory.OFFICE,
    "powerpnt.exe": BusinessCategory.OFFICE,
    "ms-teams.exe": BusinessCategory.COMMUNICATION,
    "teams.exe": BusinessCategory.COMMUNICATION,
    "slack.exe": BusinessCategory.COMMUNICATION,
    "outlook.exe": BusinessCategory.COMMUNICATION,
    "zoom.exe": BusinessCategory.COMMUNICATION,
    "explorer.exe": BusinessCategory.SYSTEM,
    "searchhost.exe": BusinessCategory.SYSTEM,
    "lockapp.exe": BusinessCategory.SYSTEM,
    "dwm.exe": BusinessCategory.SYSTEM,
    "applicationframehost.exe": BusinessCategory.SYSTEM,
    "systemsettings.exe": BusinessCategory.SYSTEM,
    "logonui.exe": BusinessCategory.SYSTEM,
    "shellexperiencehost.exe": BusinessCategory.SYSTEM,
    "sihost.exe": BusinessCategory.SYSTEM,
    "textinputhost.exe": BusinessCategory.SYSTEM,
}

_HOST_CATEGORY: dict[str, BusinessCategory] = {
    "github.com": BusinessCategory.DEVELOPMENT,
    "gitlab.com": BusinessCategory.DEVELOPMENT,
    "office.com": BusinessCategory.OFFICE,
    "microsoft365.com": BusinessCategory.OFFICE,
    "sharepoint.com": BusinessCategory.OFFICE,
    "docs.google.com": BusinessCategory.OFFICE,
    "jira.company.com": BusinessCategory.PROJECT_MANAGEMENT,
    "atlassian.net": BusinessCategory.PROJECT_MANAGEMENT,
    "chat.openai.com": BusinessCategory.AI_ASSISTANT,
    "chatgpt.com": BusinessCategory.AI_ASSISTANT,
    "openai.com": BusinessCategory.AI_ASSISTANT,
    "claude.ai": BusinessCategory.AI_ASSISTANT,
    "gemini.google.com": BusinessCategory.AI_ASSISTANT,
    "salesforce.com": BusinessCategory.CRM,
    "force.com": BusinessCategory.CRM,
    "google.com": BusinessCategory.RESEARCH,
    "youtube.com": BusinessCategory.ENTERTAINMENT,
    "facebook.com": BusinessCategory.SOCIAL_MEDIA,
    "linkedin.com": BusinessCategory.SOCIAL_MEDIA,
    "netflix.com": BusinessCategory.STREAMING,
    "dropbox.com": BusinessCategory.FILE_SHARING,
    "mega.nz": BusinessCategory.FILE_SHARING,
    "mega.io": BusinessCategory.FILE_SHARING,
    "wetransfer.com": BusinessCategory.FILE_SHARING,
    "drive.google.com": BusinessCategory.FILE_SHARING,
    "internal.company.com": BusinessCategory.OTHER,
}

_CATEGORY_PRODUCTIVITY: dict[BusinessCategory, ProductivityClass] = {
    BusinessCategory.DEVELOPMENT: ProductivityClass.PRODUCTIVE,
    BusinessCategory.OFFICE: ProductivityClass.PRODUCTIVE,
    BusinessCategory.COMMUNICATION: ProductivityClass.PRODUCTIVE,
    BusinessCategory.PROJECT_MANAGEMENT: ProductivityClass.PRODUCTIVE,
    BusinessCategory.CRM: ProductivityClass.PRODUCTIVE,
    BusinessCategory.ERP: ProductivityClass.PRODUCTIVE,
    BusinessCategory.FINANCE: ProductivityClass.PRODUCTIVE,
    BusinessCategory.HR: ProductivityClass.PRODUCTIVE,
    BusinessCategory.AI_ASSISTANT: ProductivityClass.PRODUCTIVE,
    BusinessCategory.RESEARCH: ProductivityClass.NEUTRAL,
    BusinessCategory.BROWSER: ProductivityClass.NEUTRAL,
    BusinessCategory.ENTERTAINMENT: ProductivityClass.UNPRODUCTIVE,
    BusinessCategory.SOCIAL_MEDIA: ProductivityClass.UNPRODUCTIVE,
    BusinessCategory.STREAMING: ProductivityClass.UNPRODUCTIVE,
    BusinessCategory.GAMING: ProductivityClass.UNPRODUCTIVE,
    BusinessCategory.FILE_SHARING: ProductivityClass.NEUTRAL,
    BusinessCategory.SYSTEM: ProductivityClass.IGNORED,
    BusinessCategory.OTHER: ProductivityClass.NEUTRAL,
}

_CATEGORY_RISK: dict[BusinessCategory, RiskLevel] = {
    BusinessCategory.DEVELOPMENT: RiskLevel.LOW,
    BusinessCategory.OFFICE: RiskLevel.LOW,
    BusinessCategory.COMMUNICATION: RiskLevel.LOW,
    BusinessCategory.PROJECT_MANAGEMENT: RiskLevel.LOW,
    BusinessCategory.CRM: RiskLevel.LOW,
    BusinessCategory.ERP: RiskLevel.LOW,
    BusinessCategory.FINANCE: RiskLevel.LOW,
    BusinessCategory.HR: RiskLevel.LOW,
    BusinessCategory.AI_ASSISTANT: RiskLevel.MEDIUM,
    BusinessCategory.RESEARCH: RiskLevel.LOW,
    BusinessCategory.BROWSER: RiskLevel.MEDIUM,
    BusinessCategory.ENTERTAINMENT: RiskLevel.LOW,
    BusinessCategory.SOCIAL_MEDIA: RiskLevel.LOW,
    BusinessCategory.STREAMING: RiskLevel.LOW,
    BusinessCategory.GAMING: RiskLevel.LOW,
    BusinessCategory.FILE_SHARING: RiskLevel.MEDIUM,
    BusinessCategory.SYSTEM: RiskLevel.LOW,
    BusinessCategory.OTHER: RiskLevel.MEDIUM,
}

_HOST_RISK: dict[str, RiskLevel] = {
    "mega.nz": RiskLevel.HIGH,
    "mega.io": RiskLevel.HIGH,
}


class PlatformSeedMappingProvider:
    """Shipped platform defaults. Replace this class with a DB-backed provider later."""

    def is_browser_container(self, executable_key: str) -> bool:
        return executable_key in _BROWSER_CONTAINERS

    def category_for_executable(self, executable_key: str) -> BusinessCategory | None:
        return _EXECUTABLE_CATEGORY.get(executable_key)

    def category_for_host(self, host: str) -> BusinessCategory | None:
        return _HOST_CATEGORY.get(host)

    def productivity_for_category(self, category: BusinessCategory) -> ProductivityClass:
        return _CATEGORY_PRODUCTIVITY[category]

    def risk_for_category(self, category: BusinessCategory) -> RiskLevel:
        return _CATEGORY_RISK[category]

    def productivity_for_host(self, host: str) -> ProductivityClass | None:
        return None

    def risk_for_host(self, host: str) -> RiskLevel | None:
        return _HOST_RISK.get(host)
