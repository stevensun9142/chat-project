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

// pushIfExists is a Lua script that only appends to the list when the key
// already exists. This avoids creating a partial cache entry for rooms that
// haven't been cached yet (the read path is responsible for populating the
// cache from Cassandra).
var pushIfExists = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 1 then
  redis.call("RPUSH", KEYS[1], ARGV[1])
  redis.call("LTRIM", KEYS[1], -tonumber(ARGV[2]), -1)
  redis.call("EXPIRE", KEYS[1], ARGV[3])
  return 1
end
return 0
`)

// PushMessage appends a message to the room's cache list only if the key
// already exists. If the room isn't cached, this is a no-op — the read path
// is responsible for populating the cache from Cassandra. Best-effort: errors
// are logged by the caller but do not block the consumer.
func (r *Redis) PushMessage(ctx context.Context, msg CachedMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	key := cachePrefix + msg.RoomID
	_, err = pushIfExists.Run(ctx, r.rdb, []string{key},
		data, cacheLimit, int(cacheTTL.Seconds()),
	).Result()
	return err
}

// Evict deletes the cached message list for a room. Called when a cache write
// fails after a successful Cassandra persist — forces the next read to
// repopulate from Cassandra rather than serving stale data.
func (r *Redis) Evict(ctx context.Context, roomID string) error {
	return r.rdb.Del(ctx, cachePrefix+roomID).Err()
}
