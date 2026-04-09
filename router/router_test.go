package main_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"

	"github.com/stevensun/chat-project/router/delivery"
	"github.com/stevensun/chat-project/router/presence"
	"github.com/stevensun/chat-project/router/store"

	pb "github.com/stevensun/chat-project/proto"
	"google.golang.org/grpc"
)

var kafkaBrokers = []string{"localhost:9092", "localhost:9093", "localhost:9094"}

const (
	testPgDSN    = "postgres://chat:chat_secret@localhost:5432/chat_db_test?sslmode=disable"
	testRedis    = "localhost:6379"
	pollInterval = 200 * time.Millisecond
	pollTimeout  = 10 * time.Second
)

// --- Mock gRPC gateway ---

// mockGateway implements the Delivery gRPC service and records delivered messages.
type mockGateway struct {
	pb.UnimplementedDeliveryServer
	mu       sync.Mutex
	received []*pb.DeliverMessage
}

func (m *mockGateway) Deliver(_ context.Context, req *pb.DeliverRequest) (*pb.DeliverResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.received = append(m.received, req.Messages...)
	return &pb.DeliverResponse{Delivered: int32(len(req.Messages))}, nil
}

func (m *mockGateway) messages() []*pb.DeliverMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]*pb.DeliverMessage, len(m.received))
	copy(cp, m.received)
	return cp
}

// startMockGateway starts a gRPC server and returns the address and cleanup func.
func startMockGateway(t *testing.T) (*mockGateway, string) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	mock := &mockGateway{}
	gs := grpc.NewServer()
	pb.RegisterDeliveryServer(gs, mock)
	go gs.Serve(lis)
	t.Cleanup(func() { gs.Stop() })
	return mock, lis.Addr().String()
}

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

// --- Redis helpers ---

func setupRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: testRedis})
	t.Cleanup(func() { rdb.Close() })
	return rdb
}

// --- Kafka helpers ---

func publishDelivery(t *testing.T, evt delivery.MessageEvent) {
	t.Helper()
	value, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	w := &kafka.Writer{
		Addr:         kafka.TCP(kafkaBrokers...),
		Topic:        "chat.delivery",
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
		t.Fatalf("publish delivery: %v", err)
	}
}

func publishPresence(t *testing.T, evt presence.PresenceEvent) {
	t.Helper()
	value, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	w := &kafka.Writer{
		Addr:         kafka.TCP(kafkaBrokers...),
		Topic:        "presence.events",
		Balancer:     &kafka.Hash{},
		BatchTimeout: 10 * time.Millisecond,
	}
	defer w.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.WriteMessages(ctx, kafka.Message{
		Key:   []byte(evt.UserID),
		Value: value,
	}); err != nil {
		t.Fatalf("publish presence: %v", err)
	}
}

// startPresenceConsumer starts the presence consumer in a goroutine.
func startPresenceConsumer(t *testing.T, groupID string, registry *presence.Registry) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	c := presence.NewConsumer(kafkaBrokers, groupID, registry)
	go func() {
		if err := c.Run(ctx); err != nil {
			t.Logf("presence consumer exited: %v", err)
		}
	}()
	return cancel
}

// startDeliveryConsumer starts the delivery consumer in a goroutine.
func startDeliveryConsumer(t *testing.T, groupID string, pg *store.Postgres, registry *presence.Registry, batcher *delivery.Batcher) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	c := delivery.NewConsumer(kafkaBrokers, groupID, pg, registry, batcher)
	go func() {
		if err := c.Run(ctx); err != nil {
			t.Logf("delivery consumer exited: %v", err)
		}
	}()
	return cancel
}

// --- Tests ---

func TestPresenceUpdatesRedis(t *testing.T) {
	rdb := setupRedis(t)
	registry := presence.NewRegistry(rdb)

	userID := "aa000000-0000-0000-0000-000000000001"
	gwID := "test-gateway-0"
	redisKey := "presence:" + userID

	t.Cleanup(func() { rdb.Del(context.Background(), redisKey) })

	groupID := fmt.Sprintf("test-presence-%d", time.Now().UnixNano())
	cancel := startPresenceConsumer(t, groupID, registry)
	defer cancel()
	time.Sleep(2 * time.Second) // let consumer join group

	// Publish connect event.
	publishPresence(t, presence.PresenceEvent{
		UserID:    userID,
		Username:  "presenceuser",
		GatewayID: gwID,
		Event:     "connect",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})

	// Poll Redis until the key appears.
	deadline := time.Now().Add(pollTimeout)
	for time.Now().Before(deadline) {
		val, err := rdb.Get(context.Background(), redisKey).Result()
		if err == nil && val == gwID {
			break
		}
		time.Sleep(pollInterval)
	}
	val, err := rdb.Get(context.Background(), redisKey).Result()
	if err != nil {
		t.Fatalf("redis GET after connect: %v", err)
	}
	if val != gwID {
		t.Errorf("presence value: got %q, want %q", val, gwID)
	}

	// Publish disconnect event.
	publishPresence(t, presence.PresenceEvent{
		UserID:    userID,
		Username:  "presenceuser",
		GatewayID: gwID,
		Event:     "disconnect",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})

	// Poll Redis until the key is gone.
	deadline = time.Now().Add(pollTimeout)
	for time.Now().Before(deadline) {
		_, err := rdb.Get(context.Background(), redisKey).Result()
		if err == redis.Nil {
			return // success
		}
		time.Sleep(pollInterval)
	}
	t.Fatal("presence key not deleted after disconnect within timeout")
}

func TestDeliverToOnlineMember(t *testing.T) {
	db := setupPg(t)
	rdb := setupRedis(t)

	userID := "bb000000-0000-0000-0000-000000000001"
	senderID := "bb000000-0000-0000-0000-000000000002"
	roomID := "bb000000-0000-0000-0000-0000000000f1"
	gwID := "mock-gw-0"

	// Postgres: create users, room, membership.
	createTestUser(t, db, userID, "deliveruser1")
	createTestUser(t, db, senderID, "deliversender1")
	createTestRoom(t, db, roomID, senderID)
	addTestMember(t, db, roomID, userID)
	addTestMember(t, db, roomID, senderID)
	t.Cleanup(func() {
		cleanupPg(t, db, roomID, []string{userID, senderID})
	})

	// Redis: mark user as online on the mock gateway.
	rdb.Set(context.Background(), "presence:"+userID, gwID, 90*time.Second)
	rdb.Set(context.Background(), "presence:"+senderID, gwID, 90*time.Second)
	t.Cleanup(func() {
		rdb.Del(context.Background(), "presence:"+userID, "presence:"+senderID)
	})

	// Start mock gRPC gateway.
	mock, addr := startMockGateway(t)

	// Wire up the delivery consumer.
	pg, err := store.NewPostgres(testPgDSN)
	if err != nil {
		t.Fatalf("store postgres: %v", err)
	}
	t.Cleanup(func() { pg.Close() })

	registry := presence.NewRegistry(rdb)
	pool := delivery.NewGatewayPool(map[string]string{gwID: addr})
	t.Cleanup(func() { pool.Close() })
	batcher := delivery.NewBatcher(pool)
	t.Cleanup(func() { batcher.Close() })

	groupID := fmt.Sprintf("test-deliver-%d", time.Now().UnixNano())
	cancel := startDeliveryConsumer(t, groupID, pg, registry, batcher)
	defer cancel()
	time.Sleep(2 * time.Second)

	// Publish a delivery event.
	now := time.Now().UTC()
	msgID := now.UnixMilli()<<22 | 1
	publishDelivery(t, delivery.MessageEvent{
		MessageID:  msgID,
		RoomID:     roomID,
		SenderID:   senderID,
		SenderName: "deliversender1",
		Content:    "hello from router test",
		CreatedAt:  now.Format(time.RFC3339),
	})

	// Poll mock gateway until it receives the message.
	deadline := time.Now().Add(pollTimeout)
	for time.Now().Before(deadline) {
		msgs := mock.messages()
		for _, m := range msgs {
			if m.MessageId == msgID && m.Content == "hello from router test" {
				// Verify user_ids includes both members.
				if len(m.UserIds) != 2 {
					t.Errorf("user_ids count: got %d, want 2", len(m.UserIds))
				}
				return // success
			}
		}
		time.Sleep(pollInterval)
	}
	t.Fatal("mock gateway did not receive delivered message within timeout")
}

func TestSkipOfflineMembers(t *testing.T) {
	db := setupPg(t)
	rdb := setupRedis(t)

	onlineUser := "cc000000-0000-0000-0000-000000000001"
	offlineUser := "cc000000-0000-0000-0000-000000000002"
	senderID := "cc000000-0000-0000-0000-000000000003"
	roomID := "cc000000-0000-0000-0000-0000000000f1"
	gwID := "mock-gw-1"

	// Postgres: all three users in the room.
	createTestUser(t, db, onlineUser, "onlineuser")
	createTestUser(t, db, offlineUser, "offlineuser")
	createTestUser(t, db, senderID, "sender3")
	createTestRoom(t, db, roomID, senderID)
	addTestMember(t, db, roomID, onlineUser)
	addTestMember(t, db, roomID, offlineUser)
	addTestMember(t, db, roomID, senderID)
	t.Cleanup(func() {
		cleanupPg(t, db, roomID, []string{onlineUser, offlineUser, senderID})
	})

	// Redis: only onlineUser and sender are online; offlineUser has no presence key.
	rdb.Set(context.Background(), "presence:"+onlineUser, gwID, 90*time.Second)
	rdb.Set(context.Background(), "presence:"+senderID, gwID, 90*time.Second)
	t.Cleanup(func() {
		rdb.Del(context.Background(), "presence:"+onlineUser, "presence:"+senderID)
	})

	mock, addr := startMockGateway(t)

	pg, err := store.NewPostgres(testPgDSN)
	if err != nil {
		t.Fatalf("store postgres: %v", err)
	}
	t.Cleanup(func() { pg.Close() })

	registry := presence.NewRegistry(rdb)
	pool := delivery.NewGatewayPool(map[string]string{gwID: addr})
	t.Cleanup(func() { pool.Close() })
	batcher := delivery.NewBatcher(pool)
	t.Cleanup(func() { batcher.Close() })

	groupID := fmt.Sprintf("test-offline-%d", time.Now().UnixNano())
	cancel := startDeliveryConsumer(t, groupID, pg, registry, batcher)
	defer cancel()
	time.Sleep(2 * time.Second)

	// Publish delivery event.
	now := time.Now().UTC()
	msgID := now.UnixMilli()<<22 | 2
	publishDelivery(t, delivery.MessageEvent{
		MessageID:  msgID,
		RoomID:     roomID,
		SenderID:   senderID,
		SenderName: "sender3",
		Content:    "offline test",
		CreatedAt:  now.Format(time.RFC3339),
	})

	// Poll mock gateway.
	deadline := time.Now().Add(pollTimeout)
	for time.Now().Before(deadline) {
		msgs := mock.messages()
		for _, m := range msgs {
			if m.MessageId == msgID {
				// Should include onlineUser + senderID but NOT offlineUser.
				userSet := make(map[string]bool)
				for _, uid := range m.UserIds {
					userSet[uid] = true
				}
				if !userSet[onlineUser] {
					t.Errorf("expected onlineUser %s in user_ids", onlineUser)
				}
				if !userSet[senderID] {
					t.Errorf("expected sender %s in user_ids", senderID)
				}
				if userSet[offlineUser] {
					t.Errorf("offlineUser %s should NOT be in user_ids", offlineUser)
				}
				if len(m.UserIds) != 2 {
					t.Errorf("user_ids count: got %d, want 2", len(m.UserIds))
				}
				return // success
			}
		}
		time.Sleep(pollInterval)
	}
	t.Fatal("mock gateway did not receive delivered message within timeout")
}
