package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

// MessageEvent is the payload published to chat.messages and chat.delivery.
type MessageEvent struct {
	RoomID     string `json:"room_id"`
	SenderID   string `json:"sender_id"`
	SenderName string `json:"sender_name"`
	Content    string `json:"content"`
	CreatedAt  string `json:"created_at"`
}

// PresenceEvent is the payload published to presence.events.
type PresenceEvent struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	GatewayID string `json:"gateway_id"`
	Event     string `json:"event"` // "connect" or "disconnect"
	Timestamp string `json:"timestamp"`
}

// Producer publishes message events to Kafka.
type Producer struct {
	messages  *kafka.Writer
	delivery  *kafka.Writer
	presence  *kafka.Writer
	GatewayID string
}

func NewProducer(brokers []string, gatewayID string) *Producer {
	return &Producer{
		GatewayID: gatewayID,
		messages: &kafka.Writer{
			Addr:         kafka.TCP(brokers...),
			Topic:        "chat.messages",
			Balancer:     &kafka.Hash{},
			RequiredAcks: kafka.RequireAll,
			BatchTimeout: 10 * time.Millisecond,
		},
		delivery: &kafka.Writer{
			Addr:         kafka.TCP(brokers...),
			Topic:        "chat.delivery",
			Balancer:     &kafka.Hash{},
			RequiredAcks: kafka.RequireOne,
			BatchTimeout: 10 * time.Millisecond,
		},
		presence: &kafka.Writer{
			Addr:         kafka.TCP(brokers...),
			Topic:        "presence.events",
			Balancer:     &kafka.Hash{},
			RequiredAcks: kafka.RequireOne,
			BatchTimeout: 10 * time.Millisecond,
		},
	}
}

// Publish writes a message event to both chat.messages and chat.delivery topics.
func (p *Producer) Publish(ctx context.Context, roomID, senderID, senderName, content string) error {
	event := MessageEvent{
		RoomID:     roomID,
		SenderID:   senderID,
		SenderName: senderName,
		Content:    content,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	value, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal message event: %w", err)
	}

	key := []byte(roomID)

	// Publish to chat.messages (persistence path — needs all replicas to ack)
	if err := p.messages.WriteMessages(ctx, kafka.Message{
		Key:   key,
		Value: value,
	}); err != nil {
		return fmt.Errorf("publish to chat.messages: %w", err)
	}

	// Publish to chat.delivery (real-time path — one ack is enough for low latency)
	if err := p.delivery.WriteMessages(ctx, kafka.Message{
		Key:   key,
		Value: value,
	}); err != nil {
		return fmt.Errorf("publish to chat.delivery: %w", err)
	}

	return nil
}

// PublishPresence writes a connect or disconnect event to presence.events.
func (p *Producer) PublishPresence(ctx context.Context, userID, username, event string) error {
	evt := PresenceEvent{
		UserID:    userID,
		Username:  username,
		GatewayID: p.GatewayID,
		Event:     event,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	value, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal presence event: %w", err)
	}

	if err := p.presence.WriteMessages(ctx, kafka.Message{
		Key:   []byte(userID),
		Value: value,
	}); err != nil {
		return fmt.Errorf("publish to presence.events: %w", err)
	}

	return nil
}

func (p *Producer) Close() error {
	var errs []error
	if err := p.messages.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close messages writer: %w", err))
	}
	if err := p.delivery.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close delivery writer: %w", err))
	}
	if err := p.presence.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close presence writer: %w", err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("%v", errs)
	}
	return nil
}
