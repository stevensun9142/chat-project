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

// Producer publishes message events to Kafka.
type Producer struct {
	messages *kafka.Writer
	delivery *kafka.Writer
}

func NewProducer(brokers []string) *Producer {
	return &Producer{
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

func (p *Producer) Close() error {
	var errs []error
	if err := p.messages.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close messages writer: %w", err))
	}
	if err := p.delivery.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close delivery writer: %w", err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("%v", errs)
	}
	return nil
}
