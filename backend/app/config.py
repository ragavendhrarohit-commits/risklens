from pathlib import Path

from pydantic_settings import BaseSettings, SettingsConfigDict

ROOT = Path(__file__).resolve().parents[2]


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=str(ROOT / ".env"),
        env_file_encoding="utf-8",
        extra="ignore",
    )

    metadata_db_path: str = "backend/data/risklens.db"

    @property
    def db_path(self) -> Path:
        path = Path(self.metadata_db_path)
        if not path.is_absolute():
            path = ROOT / path
        return path


settings = Settings()
