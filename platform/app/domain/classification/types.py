from __future__ import annotations

from enum import Enum


class BusinessCategory(str, Enum):
    DEVELOPMENT = "Development"
    OFFICE = "Office"
    COMMUNICATION = "Communication"
    PROJECT_MANAGEMENT = "Project Management"
    CRM = "CRM"
    ERP = "ERP"
    FINANCE = "Finance"
    HR = "HR"
    AI_ASSISTANT = "AI Assistant"
    RESEARCH = "Research"
    BROWSER = "Browser"
    ENTERTAINMENT = "Entertainment"
    SOCIAL_MEDIA = "Social Media"
    STREAMING = "Streaming"
    GAMING = "Gaming"
    FILE_SHARING = "File Sharing"
    SYSTEM = "System"
    OTHER = "Other"


class ProductivityClass(str, Enum):
    PRODUCTIVE = "Productive"
    NEUTRAL = "Neutral"
    UNPRODUCTIVE = "Unproductive"
    IGNORED = "Ignored"


class RiskLevel(str, Enum):
    LOW = "Low"
    MEDIUM = "Medium"
    HIGH = "High"
