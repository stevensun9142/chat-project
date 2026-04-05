from app.config.settings import settings

# Point all tests at the isolated test databases
settings.pg_database = "chat_db_test"
settings.cass_keyspace = "chat_test"
