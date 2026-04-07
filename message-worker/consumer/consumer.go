package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/stevensun/chat-project/message-worker/store"
)

// MessageEvent matches the JSON published by the Gateway to chat.messages.
type MessageEvent struct {
	MessageID  int64  `json:"message_id"`
	RoomID     string `json:"room_id"`
	SenderID   string `json:"sender_id"`
	SenderName string `json:"sender_name"`
	Content    string `json:"content"`
	CreatedAt  string `json:"created_at"`
}

// Consumer reads from chat.messages and persists to Cassandra.
type Consumer struct {
	reader *kafka.Reader
	cass   *store.Cassandra
	pg     *store.Postgres
}

func New(brokers []string, groupID string, cass *store.Cassandra, pg *store.Postgres) *Consumer {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        brokers,
		Topic:          "chat.messages",
		GroupID:        groupID,
		StartOffset:    kafka.FirstOffset,
		MaxWait:        500 * time.Millisecond,
		CommitInterval: 0, // manual commit
	})
	return &Consumer{reader: r, cass: cass, pg: pg}
}

const maxRetries = 3

// Run consumes messages until the context is cancelled.
func (c *Consumer) Run(ctx context.Context) error {
	defer c.reader.Close()

	var retries int
	var lastOffset int64 = -1

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil // graceful shutdown
			}
			return fmt.Errorf("fetch: %w", err)
		}

		// Reset retry counter when we get a new message
		if msg.Offset != lastOffset {
			retries = 0
			lastOffset = msg.Offset
		}

		if err := c.handle(ctx, msg); err != nil {
			retries++
			if retries >= maxRetries {
				log.Printf("dropping message offset=%d after %d retries: %v", msg.Offset, retries, err)
			} else {
				log.Printf("handle error offset=%d attempt=%d: %v", msg.Offset, retries, err)
				time.Sleep(250 * time.Millisecond)
				continue
			}
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("commit error offset=%d: %v", msg.Offset, err)
		}
	}
}

func (c *Consumer) handle(ctx context.Context, msg kafka.Message) error {
	var evt MessageEvent
	if err := json.Unmarshal(msg.Value, &evt); err != nil {
		log.Printf("invalid JSON at offset=%d, skipping: %v", msg.Offset, err)
		return nil // skip malformed — no point retrying
	}

	if evt.RoomID == "" || evt.SenderID == "" || evt.Content == "" || evt.MessageID == 0 {
		log.Printf("missing fields at offset=%d, skipping", msg.Offset)
		return nil
	}

	member, err := c.pg.IsRoomMember(ctx, evt.RoomID, evt.SenderID)
	if err != nil {
		return fmt.Errorf("membership check: %w", err)
	}
	if !member {
		log.Printf("rejected non-member user=%s room=%s", evt.SenderID, evt.RoomID)
		return nil // skip — don't persist
	}

	createdAt, err := time.Parse(time.RFC3339, evt.CreatedAt)
	if err != nil {
		log.Printf("bad timestamp at offset=%d: %v", msg.Offset, err)
		createdAt = time.Now().UTC()
	}

	if err := c.cass.PersistMessage(evt.RoomID, evt.MessageID, evt.SenderID, evt.Content, createdAt); err != nil {
		return fmt.Errorf("cassandra write: %w", err)
	}

	log.Printf("persisted msg=%d room=%s sender=%s", evt.MessageID, evt.RoomID, evt.SenderID)
	return nil
}
