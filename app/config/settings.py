from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    # Postgres
    pg_host: str = "localhost"
    pg_port: int = 5432
    pg_user: str = "chat"
    pg_password: str = "chat_secret"
    pg_database: str = "chat_db"

    # Cassandra
    cass_hosts: list[str] = ["localhost"]
    cass_port: int = 9042
    cass_keyspace: str = "chat"

    # Redis cache
    redis_cache_url: str = "redis://localhost:6380/0"

    # Auth
    jwt_secret: str = "change-me-in-prod"
    jwt_expiry_minutes: int = 30
    refresh_token_expiry_days: int = 7

    # CORS
    cors_origins: list[str] = ["http://localhost:5173", "http://localhost:5174"]

    @property
    def pg_dsn(self) -> str:
        return f"postgresql://{self.pg_user}:{self.pg_password}@{self.pg_host}:{self.pg_port}/{self.pg_database}"


settings = Settings()
