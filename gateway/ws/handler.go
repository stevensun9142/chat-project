package ws

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/stevensun/chat-project/gateway/auth"
	"github.com/stevensun/chat-project/gateway/kafka"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: restrict to allowed origins in production
		return true
	},
}

func HandleUpgrade(hub *Hub, validator *auth.JWTValidator, producer *kafka.Producer) http.HandlerFunc {
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

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("upgrade failed: %v", err)
			return
		}

		client := NewClient(hub, conn, producer, claims.UserID, claims.Username)
		hub.Register(client)

		log.Printf("user %s (%s) connected", claims.Username, claims.UserID)

		go client.writePump()
		client.readPump() // blocks until disconnect
	}
}
