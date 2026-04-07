package presence

import (
	"context"
	"encoding/json"
	"log"

	"github.com/segmentio/kafka-go"
)

// PresenceEvent matches the JSON published by the Gateway to presence.events.
type PresenceEvent struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	GatewayID string `json:"gateway_id"`
	Event     string `json:"event"` // "connect" or "disconnect"
	Timestamp string `json:"timestamp"`
}

// Consumer reads presence events from Kafka and updates the registry.
type Consumer struct {
	reader   *kafka.Reader
	registry *Registry
}

func NewConsumer(brokers []string, groupID string, registry *Registry) *Consumer {
	return &Consumer{
		registry: registry,
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:  brokers,
			Topic:    "presence.events",
			GroupID:  groupID,
			MinBytes: 1,
			MaxBytes: 1e6,
		}),
	}
}

func (c *Consumer) Run(ctx context.Context) error {
	defer c.reader.Close()

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil // graceful shutdown
			}
			return err
		}

		var event PresenceEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			log.Printf("presence: skip malformed event: %v", err)
			c.reader.CommitMessages(ctx, msg)
			continue
		}

		switch event.Event {
		case "connect":
			if err := c.registry.Connect(ctx, event.UserID, event.GatewayID); err != nil {
				log.Printf("presence: redis connect error user=%s: %v", event.UserID, err)
			} else {
				log.Printf("presence: %s connected on %s", event.Username, event.GatewayID)
			}
		case "disconnect":
			if err := c.registry.Disconnect(ctx, event.UserID, event.GatewayID); err != nil {
				log.Printf("presence: redis disconnect error user=%s: %v", event.UserID, err)
			} else {
				log.Printf("presence: %s disconnected from %s", event.Username, event.GatewayID)
			}
		default:
			log.Printf("presence: unknown event type: %s", event.Event)
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
}
