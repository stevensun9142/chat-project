package ws

import (
	"context"
	"log"
	"math"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stevensun/chat-project/gateway/auth"
	"github.com/stevensun/chat-project/gateway/id"
	"github.com/stevensun/chat-project/gateway/kafka"
)

const presenceTimeout = 60 * time.Second

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: restrict to allowed origins in production
		return true
	},
}

func HandleUpgrade(hub *Hub, validator *auth.JWTValidator, producer *kafka.Producer, idgen *id.Generator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}

		claims, err := validator.Validate(token)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Pre-flight: publish presence before upgrading HTTP → WS.
		// If Kafka is down, the client gets a clean HTTP 503 and retries.
		if err := publishPresenceWithRetry(producer, claims.UserID, claims.Username, "connect"); err != nil {
			log.Printf("presence connect failed after retries user=%s: %v", claims.UserID, err)
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("upgrade failed: %v", err)
			return
		}

		client := NewClient(hub, conn, producer, idgen, claims.UserID, claims.Username)
		hub.Register(client)

		log.Printf("user %s (%s) connected", claims.Username, claims.UserID)

		go client.writePump()
		client.readPump() // blocks until disconnect
	}
}

// publishPresenceWithRetry retries PublishPresence with exponential backoff + jitter
// within a 60s window. Backoff: 100ms, 200ms, 400ms, ... capped at 5s per attempt.
func publishPresenceWithRetry(producer *kafka.Producer, userID, username, event string) error {
	deadline := time.Now().Add(presenceTimeout)
	baseDelay := 100 * time.Millisecond
	maxDelay := 5 * time.Second

	for attempt := 0; ; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := producer.PublishPresence(ctx, userID, username, event)
		cancel()

		if err == nil {
			return nil
		}

		if time.Now().After(deadline) {
			return err
		}

		// Exponential backoff with jitter
		delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt)))
		if delay > maxDelay {
			delay = maxDelay
		}
		jitter := time.Duration(rand.Int64N(int64(delay) / 2))
		delay = delay/2 + jitter

		log.Printf("presence %s retry attempt=%d user=%s delay=%v: %v", event, attempt+1, userID, delay, err)

		time.Sleep(delay)
	}
}
