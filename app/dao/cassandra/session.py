from cassandra.cluster import Cluster, Session

from app.config import settings

_session: Session | None = None


def get_session() -> Session:
    global _session
    if _session is None:
        cluster = Cluster(
            contact_points=settings.cass_hosts,
            port=settings.cass_port,
        )
        _session = cluster.connect(settings.cass_keyspace)
    return _session


def close_session() -> None:
    global _session
    if _session:
        _session.cluster.shutdown()
        _session = None
