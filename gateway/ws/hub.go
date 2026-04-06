package ws

import (
	"sync"

	"github.com/gorilla/websocket"
)

// Hub maintains the set of active connections indexed by user ID.
type Hub struct {
	mu    sync.RWMutex
	conns map[string]*websocket.Conn // user_id -> conn
}

func NewHub() *Hub {
	return &Hub{
		conns: make(map[string]*websocket.Conn),
	}
}

func (h *Hub) Register(userID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Close existing connection for the same user (single session)
	if old, ok := h.conns[userID]; ok {
		old.Close()
	}
	h.conns[userID] = conn
}

// Unregister removes a connection from the hub only if it matches the given
// pointer. Each read goroutine holds a reference to its own conn from spawn
// time. On reconnect, Register replaces the old conn with a new one — but the
// old goroutine still runs its cleanup defer with the old pointer. Without this
// check, the old goroutine would delete the new connection from the map.
func (h *Hub) Unregister(userID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conns[userID] == conn {
		delete(h.conns, userID)
	}
}

func (h *Hub) Get(userID string) (*websocket.Conn, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conn, ok := h.conns[userID]
	return conn, ok
}
