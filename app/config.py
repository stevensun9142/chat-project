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

    @property
    def pg_dsn(self) -> str:
        return f"postgresql://{self.pg_user}:{self.pg_password}@{self.pg_host}:{self.pg_port}/{self.pg_database}"


settings = Settings()
