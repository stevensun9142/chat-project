package ws

// ClientMessage is sent from the browser to the gateway.
type ClientMessage struct {
	Type    string `json:"type"`     // "send_message"
	RoomID  string `json:"room_id"`
	Content string `json:"content"`
}

// ServerMessage is sent from the gateway to the browser.
type ServerMessage struct {
	Type       string `json:"type"`        // "new_message", "error"
	MessageID  int64  `json:"message_id,omitempty"`
	RoomID     string `json:"room_id,omitempty"`
	SenderID   string `json:"sender_id,omitempty"`
	SenderName string `json:"sender_name,omitempty"`
	Content    string `json:"content,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	Message    string `json:"message,omitempty"` // for error type
}
