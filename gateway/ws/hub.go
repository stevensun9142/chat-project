package ws

import (
	"log"
	"sync"
)

// Hub maintains the set of active clients indexed by user ID.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]*Client // user_id -> client
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]*Client),
	}
}

func (h *Hub) Register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if old, ok := h.clients[client.UserID]; ok {
		log.Printf("replacing connection for user=%s", client.UserID)
		close(old.send)
	}
	h.clients[client.UserID] = client
}

func (h *Hub) Unregister(userID string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[userID] == client {
		close(client.send)
		delete(h.clients, userID)
	}
}

func (h *Hub) Get(userID string) (*Client, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	client, ok := h.clients[userID]
	return client, ok
}
