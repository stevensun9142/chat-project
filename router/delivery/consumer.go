package delivery

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/stevensun/chat-project/router/presence"
	"github.com/stevensun/chat-project/router/store"

	pb "github.com/stevensun/chat-project/proto"
)

// MessageEvent matches the JSON published by the Gateway to chat.delivery.
type MessageEvent struct {
	MessageID  int64  `json:"message_id"`
	RoomID     string `json:"room_id"`
	SenderID   string `json:"sender_id"`
	SenderName string `json:"sender_name"`
	Content    string `json:"content"`
	CreatedAt  string `json:"created_at"`
}

// Consumer reads from chat.delivery, resolves recipients, and fans out via gRPC.
type Consumer struct {
	reader   *kafka.Reader
	pg       *store.Postgres
	registry *presence.Registry
	pool     *GatewayPool
}

const maxRetries = 3

func NewConsumer(brokers []string, groupID string, pg *store.Postgres, registry *presence.Registry, pool *GatewayPool) *Consumer {
	return &Consumer{
		pg:       pg,
		registry: registry,
		pool:     pool,
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:        brokers,
			Topic:          "chat.delivery",
			GroupID:        groupID,
			StartOffset:    kafka.FirstOffset,
			MaxWait:        500 * time.Millisecond,
			CommitInterval: 0, // manual commit
		}),
	}
}

// Run consumes delivery events until the context is cancelled.
func (c *Consumer) Run(ctx context.Context) error {
	defer c.reader.Close()

	var retries int
	var lastOffset int64 = -1

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}

		// Reset retry counter when we get a new message.
		if msg.Offset != lastOffset {
			retries = 0
			lastOffset = msg.Offset
		}

		if err := c.handle(ctx, msg); err != nil {
			retries++
			if retries >= maxRetries {
				log.Printf("delivery: dropping offset=%d after %d retries: %v", msg.Offset, retries, err)
			} else {
				log.Printf("delivery: handle error offset=%d attempt=%d: %v", msg.Offset, retries, err)
				time.Sleep(250 * time.Millisecond)
				continue
			}
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("delivery: commit error offset=%d: %v", msg.Offset, err)
		}
	}
}

func (c *Consumer) handle(ctx context.Context, msg kafka.Message) error {
	var evt MessageEvent
	if err := json.Unmarshal(msg.Value, &evt); err != nil {
		log.Printf("delivery: skip malformed at offset=%d: %v", msg.Offset, err)
		return nil // no point retrying bad JSON
	}

	if evt.RoomID == "" || evt.MessageID == 0 {
		log.Printf("delivery: skip missing fields at offset=%d", msg.Offset)
		return nil
	}

	// Get all members of the room. Retryable on transient Postgres failure.
	memberIDs, err := c.pg.RoomMemberIDs(ctx, evt.RoomID)
	if err != nil {
		return fmt.Errorf("room members lookup room=%s: %w", evt.RoomID, err)
	}

	// Group online members by gateway.
	// gateway_id -> []user_id
	usersByGateway := make(map[string][]string)
	for _, userID := range memberIDs {
		gwID, err := c.registry.GatewayID(ctx, userID)
		if err != nil {
			return fmt.Errorf("registry lookup user=%s: %w", userID, err)
		}
		if gwID == "" {
			continue // user is offline
		}
		usersByGateway[gwID] = append(usersByGateway[gwID], userID)
	}

	if len(usersByGateway) == 0 {
		return nil // no online recipients
	}

	// Send one gRPC Deliver call per gateway. Best-effort — don't retry gRPC failures.
	for gwID, userIDs := range usersByGateway {
		client, err := c.pool.Client(gwID)
		if err != nil {
			log.Printf("delivery: grpc client gateway=%s: %v", gwID, err)
			continue
		}

		req := &pb.DeliverRequest{
			Messages: []*pb.DeliverMessage{
				{
					UserIds:    userIDs,
					MessageId:  evt.MessageID,
					RoomId:     evt.RoomID,
					SenderId:   evt.SenderID,
					SenderName: evt.SenderName,
					Content:    evt.Content,
					CreatedAt:  evt.CreatedAt,
				},
			},
		}

		callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		resp, err := client.Deliver(callCtx, req)
		cancel()
		if err != nil {
			log.Printf("delivery: grpc deliver gateway=%s: %v", gwID, err)
			continue
		}

		log.Printf("delivery: room=%s gateway=%s delivered=%d users=%d",
			evt.RoomID, gwID, resp.Delivered, len(userIDs))
	}

	return nil
}
