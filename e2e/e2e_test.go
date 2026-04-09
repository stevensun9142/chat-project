package e2e_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"

	"github.com/stevensun/chat-project/gateway/auth"
	gwgrpc "github.com/stevensun/chat-project/gateway/grpc"
	"github.com/stevensun/chat-project/gateway/id"
	gwkafka "github.com/stevensun/chat-project/gateway/kafka"
	"github.com/stevensun/chat-project/gateway/ws"

	"github.com/stevensun/chat-project/router/delivery"
	"github.com/stevensun/chat-project/router/presence"
	"github.com/stevensun/chat-project/router/store"

	pb "github.com/stevensun/chat-project/proto"
)

const (
	jwtSecret    = "e2e-test-secret"
	testPgDSN    = "postgres://chat:chat_secret@localhost:5432/chat_db_test?sslmode=disable"
	testRedis    = "localhost:6379"
	pollInterval = 200 * time.Millisecond
	pollTimeout  = 15 * time.Second
)

var kafkaBrokers = []string{"localhost:9092", "localhost:9093", "localhost:9094"}

// --- JWT helper ---

func signJWT(t *testing.T, userID, username string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":      userID,
		"username": username,
		"exp":      time.Now().Add(30 * time.Minute).Unix(),
	})
	s, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return s
}

// --- Postgres helpers ---

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
		"INSERT INTO rooms (id, name, created_by) VALUES ($1, 'e2e-room', $2) ON CONFLICT DO NOTHING",
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

// --- Gateway helper ---

type gatewayInstance struct {
	hub        *ws.Hub
	httpServer *httptest.Server
	grpcAddr   string
}

func startGateway(t *testing.T, gatewayID string) *gatewayInstance {
	t.Helper()

	validator := auth.NewJWTValidator(jwtSecret)
	hub := ws.NewHub()
	producer := gwkafka.NewProducer(kafkaBrokers, gatewayID)
	idgen := id.NewGenerator(gatewayID)
	t.Cleanup(func() { producer.Close() })

	// HTTP server for WebSocket connections.
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", ws.HandleUpgrade(hub, validator, producer, idgen))
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)

	// gRPC server for receiving routed messages from Router.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grpc listen: %v", err)
	}
	gs := grpc.NewServer()
	pb.RegisterDeliveryServer(gs, gwgrpc.NewServer(hub))
	go gs.Serve(lis)
	t.Cleanup(gs.Stop)

	return &gatewayInstance{
		hub:        hub,
		httpServer: httpServer,
		grpcAddr:   lis.Addr().String(),
	}
}

// --- WebSocket helper ---

func dialWS(t *testing.T, serverURL, token string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/ws?token=" + token
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial WS: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// --- Test ---

// TestEndToEndCrossGatewayDelivery verifies the full message delivery pipeline:
//
//	WS client A → Gateway A → Kafka → Router → gRPC → Gateway B → WS client B
//
// This is the Phase 6 end-to-end test described in the STEERING.
func TestEndToEndCrossGatewayDelivery(t *testing.T) {
	db := setupPg(t)

	userA := "e2e00000-0000-0000-0000-000000000001"
	userB := "e2e00000-0000-0000-0000-000000000002"
	roomID := "e2e00000-0000-0000-0000-00000000f001"
	gwAID := "e2e-gw-a"
	gwBID := "e2e-gw-b"

	// Postgres: users + room + membership.
	createTestUser(t, db, userA, "alice-e2e")
	createTestUser(t, db, userB, "bob-e2e")
	createTestRoom(t, db, roomID, userA)
	addTestMember(t, db, roomID, userA)
	addTestMember(t, db, roomID, userB)
	t.Cleanup(func() {
		cleanupPg(t, db, roomID, []string{userA, userB})
	})

	// Start two gateway instances with separate hubs and gRPC servers.
	gwA := startGateway(t, gwAID)
	gwB := startGateway(t, gwBID)

	// Redis for presence polling.
	rdb := redis.NewClient(&redis.Options{Addr: testRedis})
	t.Cleanup(func() {
		rdb.Del(context.Background(), "presence:"+userA, "presence:"+userB)
		rdb.Close()
	})

	// Start Router: presence consumer + delivery consumer.
	registry := presence.NewRegistry(rdb)
	pg, err := store.NewPostgres(testPgDSN)
	if err != nil {
		t.Fatalf("router postgres: %v", err)
	}
	t.Cleanup(func() { pg.Close() })

	pool := delivery.NewGatewayPool(map[string]string{
		gwAID: gwA.grpcAddr,
		gwBID: gwB.grpcAddr,
	})
	t.Cleanup(func() { pool.Close() })

	batcher := delivery.NewBatcher(pool)
	t.Cleanup(func() { batcher.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ts := time.Now().UnixNano()
	presenceConsumer := presence.NewConsumer(kafkaBrokers, fmt.Sprintf("e2e-presence-%d", ts), registry)
	go presenceConsumer.Run(ctx)

	deliveryConsumer := delivery.NewConsumer(kafkaBrokers, fmt.Sprintf("e2e-delivery-%d", ts), pg, registry, batcher)
	go deliveryConsumer.Run(ctx)

	time.Sleep(2 * time.Second) // let consumers join their groups

	// Connect user A to Gateway A, user B to Gateway B.
	// HandleUpgrade publishes a presence connect event before the WS upgrade.
	connA := dialWS(t, gwA.httpServer.URL, signJWT(t, userA, "alice-e2e"))
	connB := dialWS(t, gwB.httpServer.URL, signJWT(t, userB, "bob-e2e"))

	// Poll Redis until both users' presence is registered by the Router.
	deadline := time.Now().Add(pollTimeout)
	for time.Now().Before(deadline) {
		valA, _ := rdb.Get(context.Background(), "presence:"+userA).Result()
		valB, _ := rdb.Get(context.Background(), "presence:"+userB).Result()
		if valA == gwAID && valB == gwBID {
			break
		}
		time.Sleep(pollInterval)
	}
	valA, err := rdb.Get(context.Background(), "presence:"+userA).Result()
	if err != nil || valA != gwAID {
		t.Fatalf("presence user A: got %q (err=%v), want %q", valA, err, gwAID)
	}
	valB, err := rdb.Get(context.Background(), "presence:"+userB).Result()
	if err != nil || valB != gwBID {
		t.Fatalf("presence user B: got %q (err=%v), want %q", valB, err, gwBID)
	}

	// User A sends a message through their WebSocket.
	if err := connA.WriteJSON(ws.ClientMessage{
		Type:    "send_message",
		RoomID:  roomID,
		Content: "hello from gateway A",
	}); err != nil {
		t.Fatalf("write message: %v", err)
	}

	// User B should receive the message on their WebSocket via:
	// Gateway A → Kafka chat.delivery → Router → gRPC → Gateway B → WS
	connB.SetReadDeadline(time.Now().Add(pollTimeout))
	_, raw, err := connB.ReadMessage()
	if err != nil {
		t.Fatalf("read message on user B: %v", err)
	}

	var msg ws.ServerMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if msg.Type != "new_message" {
		t.Errorf("type: got %q, want new_message", msg.Type)
	}
	if msg.RoomID != roomID {
		t.Errorf("room_id: got %q, want %q", msg.RoomID, roomID)
	}
	if msg.SenderID != userA {
		t.Errorf("sender_id: got %q, want %q", msg.SenderID, userA)
	}
	if msg.SenderName != "alice-e2e" {
		t.Errorf("sender_name: got %q, want alice-e2e", msg.SenderName)
	}
	if msg.Content != "hello from gateway A" {
		t.Errorf("content: got %q, want %q", msg.Content, "hello from gateway A")
	}
	if msg.MessageID == 0 {
		t.Error("message_id should be non-zero snowflake")
	}
}
