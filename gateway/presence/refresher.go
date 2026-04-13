package presence

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	presencePrefix = "presence:"
	presenceTTL    = 90 * time.Second
	refreshTimeout = 2 * time.Second
)

// Refresher renews a single user's presence TTL in Redis.
// Designed to be called from a client's pong handler on each ping/pong cycle.
type Refresher struct {
	rdb       *redis.Client
	gatewayID string
}

func NewRefresher(redisAddr, gatewayID string) (*Refresher, error) {
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		rdb.Close()
		return nil, err
	}

	return &Refresher{rdb: rdb, gatewayID: gatewayID}, nil
}

// Refresh renews the presence TTL for a single user.
// Safe to call from any goroutine. Uses a short timeout to avoid blocking callers.
func (r *Refresher) Refresh(userID string) {
	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()
	r.rdb.Set(ctx, presencePrefix+userID, r.gatewayID, presenceTTL)
}

func (r *Refresher) Close() error {
	return r.rdb.Close()
}
