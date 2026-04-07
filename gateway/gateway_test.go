package main_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/segmentio/kafka-go"
	"github.com/stevensun/chat-project/gateway/auth"
	"github.com/stevensun/chat-project/gateway/id"
	gwkafka "github.com/stevensun/chat-project/gateway/kafka"
	"github.com/stevensun/chat-project/gateway/ws"
)

const testSecret = "test-secret-for-integration"

var kafkaBrokers = []string{"localhost:9092", "localhost:9093", "localhost:9094"}

const readTimeout = 10 * time.Second

// --- Helpers ---

func signJWT(t *testing.T, userID, username string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":      userID,
		"username": username,
		"exp":      time.Now().Add(30 * time.Minute).Unix(),
	})
	s, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return s
}

func startGateway(t *testing.T) *httptest.Server {
	t.Helper()
	validator := auth.NewJWTValidator(testSecret)
	hub := ws.NewHub()
	producer := gwkafka.NewProducer(kafkaBrokers, "test-gateway")
	idgen := id.NewGenerator("test-gateway-0")
	t.Cleanup(func() { producer.Close() })

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", ws.HandleUpgrade(hub, validator, producer, idgen))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

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

// topicSnapshot holds the end offset for each partition at a point in time.
type topicSnapshot struct {
	topic   string
	offsets map[int]int64
}

// snapshot captures the current end offsets for all partitions of a topic.
// Messages published AFTER this call will be readable via fetchNew.
func snapshot(t *testing.T, topic string) topicSnapshot {
	t.Helper()
	ctx := context.Background()

	partitions, err := kafka.DefaultDialer.LookupPartitions(ctx, "tcp", kafkaBrokers[0], topic)
	if err != nil {
		t.Fatalf("lookup partitions for %s: %v", topic, err)
	}

	offsets := make(map[int]int64)
	for _, p := range partitions {
		conn, err := kafka.DialLeader(ctx, "tcp", kafkaBrokers[0], topic, p.ID)
		if err != nil {
			t.Fatalf("dial leader %s/%d: %v", topic, p.ID, err)
		}
		offset, err := conn.ReadLastOffset()
		conn.Close()
		if err != nil {
			t.Fatalf("last offset %s/%d: %v", topic, p.ID, err)
		}
		offsets[p.ID] = offset
	}
	return topicSnapshot{topic: topic, offsets: offsets}
}

// fetchNew reads exactly count new messages published after the snapshot.
// Spawns a reader goroutine per partition; first count messages win.
func fetchNew(t *testing.T, snap topicSnapshot, count int) []kafka.Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()

	ch := make(chan kafka.Message, 64)

	for partID, startOffset := range snap.offsets {
		go func(pid int, offset int64) {
			r := kafka.NewReader(kafka.ReaderConfig{
				Brokers:   kafkaBrokers,
				Topic:     snap.topic,
				Partition: pid,
				MaxWait:   200 * time.Millisecond,
			})
			defer r.Close()
			r.SetOffset(offset)
			for {
				msg, err := r.FetchMessage(ctx)
				if err != nil {
					return
				}
				ch <- msg
			}
		}(partID, startOffset)
	}

	var messages []kafka.Message
	for len(messages) < count {
		select {
		case msg := <-ch:
			messages = append(messages, msg)
		case <-ctx.Done():
			t.Fatalf("got %d/%d messages from %s: timeout", len(messages), count, snap.topic)
		}
	}
	return messages
}

func decodePresence(t *testing.T, data []byte) gwkafka.PresenceEvent {
	t.Helper()
	var evt gwkafka.PresenceEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		t.Fatalf("unmarshal presence: %v", err)
	}
	return evt
}

func decodeMessage(t *testing.T, data []byte) gwkafka.MessageEvent {
	t.Helper()
	var evt gwkafka.MessageEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		t.Fatalf("unmarshal message: %v", err)
	}
	return evt
}

// --- Tests ---

func TestRejectMissingToken(t *testing.T) {
	server := startGateway(t)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected error for missing token")
	}
	if resp == nil {
		t.Fatal("expected HTTP response")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", resp.StatusCode)
	}
}

func TestRejectInvalidToken(t *testing.T) {
	server := startGateway(t)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=not.a.valid.jwt"
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
	if resp == nil {
		t.Fatal("expected HTTP response")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", resp.StatusCode)
	}
}

func TestConnectPublishesPresence(t *testing.T) {
	server := startGateway(t)
	snap := snapshot(t, "presence.events")

	userID := "11111111-1111-1111-1111-111111111111"
	_ = dialWS(t, server.URL, signJWT(t, userID, "alice"))

	msgs := fetchNew(t, snap, 1)
	evt := decodePresence(t, msgs[0].Value)
	if evt.UserID != userID {
		t.Errorf("user_id: got %s, want %s", evt.UserID, userID)
	}
	if evt.Username != "alice" {
		t.Errorf("username: got %s, want alice", evt.Username)
	}
	if evt.Event != "connect" {
		t.Errorf("event: got %s, want connect", evt.Event)
	}
	if evt.GatewayID != "test-gateway" {
		t.Errorf("gateway_id: got %s, want test-gateway", evt.GatewayID)
	}
}

func TestSendMessageToKafka(t *testing.T) {
	server := startGateway(t)
	msgSnap := snapshot(t, "chat.messages")
	delSnap := snapshot(t, "chat.delivery")

	userID := "22222222-2222-2222-2222-222222222222"
	roomID := "33333333-3333-3333-3333-333333333333"
	conn := dialWS(t, server.URL, signJWT(t, userID, "bob"))

	err := conn.WriteJSON(ws.ClientMessage{
		Type:    "send_message",
		RoomID:  roomID,
		Content: "integration test message",
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	// Verify chat.messages
	mMsgs := fetchNew(t, msgSnap, 1)
	msg := decodeMessage(t, mMsgs[0].Value)
	if msg.RoomID != roomID {
		t.Errorf("room_id: got %s, want %s", msg.RoomID, roomID)
	}
	if msg.SenderID != userID {
		t.Errorf("sender_id: got %s, want %s", msg.SenderID, userID)
	}
	if msg.SenderName != "bob" {
		t.Errorf("sender_name: got %s, want bob", msg.SenderName)
	}
	if msg.Content != "integration test message" {
		t.Errorf("content: got %q, want %q", msg.Content, "integration test message")
	}
	if msg.MessageID == 0 {
		t.Error("message_id should be non-zero snowflake")
	}

	// Verify chat.delivery has the same event
	dMsgs := fetchNew(t, delSnap, 1)
	del := decodeMessage(t, dMsgs[0].Value)
	if del.MessageID != msg.MessageID {
		t.Errorf("delivery message_id %d != messages message_id %d", del.MessageID, msg.MessageID)
	}
	if del.RoomID != roomID {
		t.Errorf("delivery room_id: got %s, want %s", del.RoomID, roomID)
	}
}

func TestDisconnectPublishesPresence(t *testing.T) {
	server := startGateway(t)
	snap := snapshot(t, "presence.events")

	userID := "44444444-4444-4444-4444-444444444444"
	conn := dialWS(t, server.URL, signJWT(t, userID, "charlie"))

	// Close WebSocket — readPump defer publishes disconnect
	conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	time.Sleep(500 * time.Millisecond) // let readPump exit and publish

	// Both connect and disconnect keyed by same user_id → same partition → ordered
	msgs := fetchNew(t, snap, 2)

	connect := decodePresence(t, msgs[0].Value)
	if connect.Event != "connect" {
		t.Fatalf("first event: got %s, want connect", connect.Event)
	}

	disc := decodePresence(t, msgs[1].Value)
	if disc.UserID != userID {
		t.Errorf("user_id: got %s, want %s", disc.UserID, userID)
	}
	if disc.Username != "charlie" {
		t.Errorf("username: got %s, want charlie", disc.Username)
	}
	if disc.Event != "disconnect" {
		t.Errorf("event: got %s, want disconnect", disc.Event)
	}
}
