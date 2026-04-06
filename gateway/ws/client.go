package ws

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 30 * time.Second
	maxMessageSize = 4096
	sendBufSize    = 256
)

// Client represents a single WebSocket connection with its identity and send channel.
type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	UserID   string
	Username string
}

func NewClient(hub *Hub, conn *websocket.Conn, userID, username string) *Client {
	return &Client{
		hub:      hub,
		conn:     conn,
		send:     make(chan []byte, sendBufSize),
		UserID:   userID,
		Username: username,
	}
}

// readPump reads messages from the WebSocket connection.
// It runs in the handler goroutine (blocks until disconnect).
func (c *Client) readPump() {
	defer func() {
		c.hub.Unregister(c.UserID, c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("read error user=%s: %v", c.UserID, err)
			}
			return
		}

		var msg ClientMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			c.sendError("invalid JSON")
			continue
		}

		switch msg.Type {
		case "send_message":
			c.handleSendMessage(&msg)
		default:
			c.sendError("unknown message type: " + msg.Type)
		}
	}
}

// writePump pumps messages from the send channel to the WebSocket connection.
// A single goroutine runs this per client, ensuring serialized writes.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel — send a close frame and exit.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleSendMessage processes an incoming send_message from the client.
// TODO: publish to Kafka (Step 4). For now, log and echo back.
func (c *Client) handleSendMessage(msg *ClientMessage) {
	if msg.RoomID == "" || msg.Content == "" {
		c.sendError("room_id and content are required")
		return
	}

	log.Printf("message from user=%s room=%s: %s", c.UserID, msg.RoomID, msg.Content)

	// Placeholder: echo back as a new_message so we can test the round-trip.
	// This will be replaced by Kafka producer → Router → delivery in Step 4.
	reply := ServerMessage{
		Type:       "new_message",
		RoomID:     msg.RoomID,
		SenderID:   c.UserID,
		SenderName: c.Username,
		Content:    msg.Content,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(reply)
	if err != nil {
		log.Printf("marshal error: %v", err)
		return
	}
	c.send <- data
}

// sendError sends a JSON error message to the client.
func (c *Client) sendError(message string) {
	reply := ServerMessage{
		Type:    "error",
		Message: message,
	}
	data, err := json.Marshal(reply)
	if err != nil {
		log.Printf("marshal error: %v", err)
		return
	}

	select {
	case c.send <- data:
	default:
		// Send buffer full — client is too slow, drop the error.
		log.Printf("send buffer full for user=%s, dropping error message", c.UserID)
	}
}
