package presence

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	presencePrefix = "presence:"
	presenceTTL    = 90 * time.Second
)

// disconnectScript atomically deletes a key only if its value matches.
// Prevents stale disconnects from a previous gateway after a cross-gateway reconnect.
var disconnectScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

// Registry maintains user_id → gateway_id mappings in Redis.
type Registry struct {
	rdb redis.Cmdable
}

func NewRegistry(rdb redis.Cmdable) *Registry {
	return &Registry{rdb: rdb}
}

func key(userID string) string {
	return presencePrefix + userID
}

// Connect registers a user as connected to a specific gateway with a TTL.
// The TTL acts as a safety net — if the disconnect event is lost, the key expires.
// Phase 7 adds heartbeat renewal from the Gateway to keep the TTL alive.
func (r *Registry) Connect(ctx context.Context, userID, gatewayID string) error {
	return r.rdb.Set(ctx, key(userID), gatewayID, presenceTTL).Err()
}

// Disconnect removes a user only if the gateway_id matches (atomic via Lua script).
func (r *Registry) Disconnect(ctx context.Context, userID, gatewayID string) error {
	return disconnectScript.Run(ctx, r.rdb, []string{key(userID)}, gatewayID).Err()
}

// GatewayID returns the gateway_id for a user, or empty string if not connected.
func (r *Registry) GatewayID(ctx context.Context, userID string) (string, error) {
	val, err := r.rdb.Get(ctx, key(userID)).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("redis get presence:%s: %w", userID, err)
	}
	return val, nil
}
