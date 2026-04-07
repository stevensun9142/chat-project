package store

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gocql/gocql"
)

// Cassandra handles message persistence with weekly bucket partitioning.
type Cassandra struct {
	session *gocql.Session
}

func NewCassandra(hosts []string, port, keyspace string) (*Cassandra, error) {
	p, err := strconv.Atoi(port)
	if err != nil {
		return nil, fmt.Errorf("invalid cassandra port %q: %w", port, err)
	}

	cluster := gocql.NewCluster(hosts...)
	cluster.Port = p
	cluster.Keyspace = keyspace
	cluster.Consistency = gocql.One
	cluster.ConnectTimeout = 10 * time.Second
	cluster.Timeout = 5 * time.Second

	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("cassandra session: %w", err)
	}
	return &Cassandra{session: session}, nil
}

func (c *Cassandra) Close() {
	c.session.Close()
}

// weekBucket returns an ISO week bucket string like "2026-W14".
func weekBucket(t time.Time) string {
	year, week := t.ISOWeek()
	return fmt.Sprintf("%d-W%02d", year, week)
}

// PersistMessage inserts a message into the messages table with weekly bucketing.
func (c *Cassandra) PersistMessage(roomID string, messageID int64, senderID, content string, createdAt time.Time) error {
	bucket := weekBucket(createdAt)
	return c.session.Query(
		`INSERT INTO messages (room_id, bucket, message_id, sender_id, content, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		roomID, bucket, messageID, senderID, content, createdAt,
	).Exec()
}
