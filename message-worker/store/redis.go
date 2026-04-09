package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	cachePrefix = "msg:"
	cacheTTL    = 10 * time.Minute
	cacheLimit  = 50
)

// CachedMessage is the JSON shape stored in the Redis list.
type CachedMessage struct {
	MessageID  int64  `json:"message_id"`
	RoomID     string `json:"room_id"`
	SenderID   string `json:"sender_id"`
	SenderName string `json:"sender_name"`
	Content    string `json:"content"`
	CreatedAt  string `json:"created_at"`
}

// Redis handles message cache operations.
type Redis struct {
	rdb *redis.Client
}

func NewRedis(addr string) (*Redis, error) {
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis connect: %w", err)
	}
	return &Redis{rdb: rdb}, nil
}

func (r *Redis) Close() error {
	return r.rdb.Close()
}

// PushMessage appends a message to the room's cache list, trims to the last
// cacheLimit entries, and refreshes the TTL. Best-effort: errors are logged
// by the caller but do not block the consumer.
func (r *Redis) PushMessage(ctx context.Context, msg CachedMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	key := cachePrefix + msg.RoomID
	pipe := r.rdb.Pipeline()
	pipe.RPush(ctx, key, data)
	pipe.LTrim(ctx, key, -cacheLimit, -1)
	pipe.Expire(ctx, key, cacheTTL)
	_, err = pipe.Exec(ctx)
	return err
}
