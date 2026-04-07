package main_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gocql/gocql"
	_ "github.com/lib/pq"
	"github.com/segmentio/kafka-go"
	"github.com/stevensun/chat-project/message-worker/consumer"
	"github.com/stevensun/chat-project/message-worker/store"
)

var kafkaBrokers = []string{"localhost:9092", "localhost:9093", "localhost:9094"}

const (
	testPgDSN    = "postgres://chat:chat_secret@localhost:5432/chat_db_test?sslmode=disable"
	testCassPort = "9042"
	testCassKS   = "chat_test"
	pollInterval = 200 * time.Millisecond
	pollTimeout  = 10 * time.Second
)

// --- Postgres test helpers ---

func setupPg(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("postgres", testPgDSN)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func createTestUser(t *testing.T, db *sql.DB, id, username string) {
	t.Helper()
	_, err := db.Exec(
		"INSERT INTO users (id, username, email, password_hash) VALUES ($1, $2, $3, 'fakehash') ON CONFLICT DO NOTHING",
		id, username, username+"@test.com",
	)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
}

func createTestRoom(t *testing.T, db *sql.DB, roomID, creatorID string) {
	t.Helper()
	_, err := db.Exec(
		"INSERT INTO rooms (id, name, created_by) VALUES ($1, 'test-room', $2) ON CONFLICT DO NOTHING",
		roomID, creatorID,
	)
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
}

func addTestMember(t *testing.T, db *sql.DB, roomID, userID string) {
	t.Helper()
	_, err := db.Exec(
		"INSERT INTO room_members (room_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING",
		roomID, userID,
	)
	if err != nil {
		t.Fatalf("add member: %v", err)
	}
}

func cleanupPg(t *testing.T, db *sql.DB, roomID string, userIDs []string) {
	t.Helper()
	db.Exec("DELETE FROM room_members WHERE room_id = $1", roomID)
	db.Exec("DELETE FROM rooms WHERE id = $1", roomID)
	for _, uid := range userIDs {
		db.Exec("DELETE FROM users WHERE id = $1", uid)
	}
}

// --- Cassandra test helpers ---

func setupCass(t *testing.T) *gocql.Session {
	t.Helper()
	cluster := gocql.NewCluster("localhost")
	cluster.Port = 9042
	cluster.Keyspace = testCassKS
	cluster.Consistency = gocql.One
	cluster.ConnectTimeout = 10 * time.Second
	session, err := cluster.CreateSession()
	if err != nil {
		t.Fatalf("cassandra session: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func queryMessage(session *gocql.Session, roomID string, messageID int64, bucket string) (string, bool) {
	var content string
	err := session.Query(
		"SELECT content FROM messages WHERE room_id = ? AND bucket = ? AND created_at = ? AND message_id = ?",
		roomID, bucket, time.Now(), messageID,
	).Scan(&content)
	if err != nil {
		return "", false
	}
	return content, true
}

func queryMessageByRoom(session *gocql.Session, roomID, bucket string) (int64, string, bool) {
	var msgID int64
	var content string
	err := session.Query(
		"SELECT message_id, content FROM messages WHERE room_id = ? AND bucket = ? LIMIT 1",
		roomID, bucket,
	).Scan(&msgID, &content)
	if err != nil {
		return 0, "", false
	}
	return msgID, content, true
}

func cleanupCass(session *gocql.Session, roomID, bucket string) {
	session.Query("DELETE FROM messages WHERE room_id = ? AND bucket = ?", roomID, bucket).Exec()
}

// --- Kafka helpers ---

func publishMessage(t *testing.T, evt consumer.MessageEvent) {
	t.Helper()
	value, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	w := &kafka.Writer{
		Addr:         kafka.TCP(kafkaBrokers...),
		Topic:        "chat.messages",
		Balancer:     &kafka.Hash{},
		BatchTimeout: 10 * time.Millisecond,
	}
	defer w.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.WriteMessages(ctx, kafka.Message{
		Key:   []byte(evt.RoomID),
		Value: value,
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}
}

// startWorker starts the consumer in a goroutine, returns a cancel func.
func startWorker(t *testing.T, groupID string) context.CancelFunc {
	t.Helper()
	cass, err := store.NewCassandra([]string{"localhost"}, testCassPort, testCassKS)
	if err != nil {
		t.Fatalf("worker cassandra: %v", err)
	}
	pg, err := store.NewPostgres(testPgDSN)
	if err != nil {
		cass.Close()
		t.Fatalf("worker postgres: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	c := consumer.New(kafkaBrokers, groupID, cass, pg)

	go func() {
		if err := c.Run(ctx); err != nil {
			t.Logf("worker exited: %v", err)
		}
		cass.Close()
		pg.Close()
	}()

	return cancel
}

func weekBucket(t time.Time) string {
	year, week := t.ISOWeek()
	return fmt.Sprintf("%d-W%02d", year, week)
}

// --- Tests ---

func TestMessagePersisted(t *testing.T) {
	db := setupPg(t)
	cassSession := setupCass(t)

	userID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	roomID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	now := time.Now().UTC()
	bucket := weekBucket(now)

	// Setup: create user + room + membership
	createTestUser(t, db, userID, "testuser1")
	createTestRoom(t, db, roomID, userID)
	addTestMember(t, db, roomID, userID)
	t.Cleanup(func() {
		cleanupPg(t, db, roomID, []string{userID})
		cleanupCass(cassSession, roomID, bucket)
	})

	// Start the consumer with a unique group ID
	groupID := fmt.Sprintf("test-persist-%d", time.Now().UnixNano())
	cancel := startWorker(t, groupID)
	defer cancel()
	time.Sleep(2 * time.Second) // let consumer join group

	// Publish a message
	msgID := now.UnixMilli()<<22 | 1 // fake snowflake
	publishMessage(t, consumer.MessageEvent{
		MessageID:  msgID,
		RoomID:     roomID,
		SenderID:   userID,
		SenderName: "testuser1",
		Content:    "hello from integration test",
		CreatedAt:  now.Format(time.RFC3339),
	})

	// Poll Cassandra until the message appears
	deadline := time.Now().Add(pollTimeout)
	for time.Now().Before(deadline) {
		id, content, found := queryMessageByRoom(cassSession, roomID, bucket)
		if found && id == msgID {
			if content != "hello from integration test" {
				t.Errorf("content: got %q, want %q", content, "hello from integration test")
			}
			return // success
		}
		time.Sleep(pollInterval)
	}
	t.Fatal("message not found in Cassandra within timeout")
}

func TestNonMemberRejected(t *testing.T) {
	db := setupPg(t)
	cassSession := setupCass(t)

	memberID := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	nonMemberID := "dddddddd-dddd-dddd-dddd-dddddddddddd"
	roomID := "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
	now := time.Now().UTC()
	bucket := weekBucket(now)

	// Setup: room exists, non-member user exists, but is NOT in room_members
	createTestUser(t, db, memberID, "member2")
	createTestUser(t, db, nonMemberID, "nonmember2")
	createTestRoom(t, db, roomID, memberID)
	addTestMember(t, db, roomID, memberID)
	// nonMemberID is NOT added to room
	t.Cleanup(func() {
		cleanupPg(t, db, roomID, []string{memberID, nonMemberID})
		cleanupCass(cassSession, roomID, bucket)
	})

	// Start consumer
	groupID := fmt.Sprintf("test-reject-%d", time.Now().UnixNano())
	cancel := startWorker(t, groupID)
	defer cancel()
	time.Sleep(2 * time.Second)

	// Publish a message from the non-member
	msgID := now.UnixMilli()<<22 | 2
	publishMessage(t, consumer.MessageEvent{
		MessageID:  msgID,
		RoomID:     roomID,
		SenderID:   nonMemberID,
		SenderName: "nonmember2",
		Content:    "should be rejected",
		CreatedAt:  now.Format(time.RFC3339),
	})

	// Wait and verify the message does NOT appear in Cassandra
	time.Sleep(5 * time.Second)
	_, _, found := queryMessageByRoom(cassSession, roomID, bucket)
	if found {
		t.Error("non-member message was persisted — should have been rejected")
	}
}

func TestMalformedMessageSkipped(t *testing.T) {
	db := setupPg(t)
	cassSession := setupCass(t)

	userID := "ffffffff-ffff-ffff-ffff-ffffffffffff"
	roomID := "11111111-1111-1111-1111-111111111111"
	now := time.Now().UTC()
	bucket := weekBucket(now)

	createTestUser(t, db, userID, "testuser3")
	createTestRoom(t, db, roomID, userID)
	addTestMember(t, db, roomID, userID)
	t.Cleanup(func() {
		cleanupPg(t, db, roomID, []string{userID})
		cleanupCass(cassSession, roomID, bucket)
	})

	groupID := fmt.Sprintf("test-malformed-%d", time.Now().UnixNano())
	cancel := startWorker(t, groupID)
	defer cancel()
	time.Sleep(2 * time.Second)

	// Publish a malformed message (missing content), then a valid one
	publishMessage(t, consumer.MessageEvent{
		MessageID:  0, // invalid — zero ID
		RoomID:     roomID,
		SenderID:   userID,
		SenderName: "testuser3",
		Content:    "", // empty
		CreatedAt:  now.Format(time.RFC3339),
	})

	validMsgID := now.UnixMilli()<<22 | 3
	publishMessage(t, consumer.MessageEvent{
		MessageID:  validMsgID,
		RoomID:     roomID,
		SenderID:   userID,
		SenderName: "testuser3",
		Content:    "valid after malformed",
		CreatedAt:  now.Format(time.RFC3339),
	})

	// The valid message should still be persisted (malformed was skipped, not blocking)
	deadline := time.Now().Add(pollTimeout)
	for time.Now().Before(deadline) {
		id, content, found := queryMessageByRoom(cassSession, roomID, bucket)
		if found && id == validMsgID {
			if content != "valid after malformed" {
				t.Errorf("content: got %q, want %q", content, "valid after malformed")
			}
			return
		}
		time.Sleep(pollInterval)
	}
	t.Fatal("valid message not found — malformed message may have blocked the consumer")
}
