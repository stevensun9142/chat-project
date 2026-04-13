package presence

import (
	"context"
	"io"
	"strings"
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
	rdb       redis.Cmdable
	closer    io.Closer
	gatewayID string
}

// NewRefresher creates a Refresher connected to Redis.
// addrs is a comma-separated list of addresses. If more than one address is
// provided, a Redis Cluster client is used; otherwise a single-node client.
func NewRefresher(addrs, gatewayID string) (*Refresher, error) {
	addrList := strings.Split(addrs, ",")

	var rdb redis.Cmdable
	var closer io.Closer

	if len(addrList) > 1 {
		c := redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:         addrList,
			ReadOnly:      true,
			RouteRandomly: true,
		})
		rdb = c
		closer = c
	} else {
		c := redis.NewClient(&redis.Options{Addr: addrList[0]})
		rdb = c
		closer = c
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		closer.Close()
		return nil, err
	}

	return &Refresher{rdb: rdb, closer: closer, gatewayID: gatewayID}, nil
}

// Refresh renews the presence TTL for a single user.
// Safe to call from any goroutine. Uses a short timeout to avoid blocking callers.
func (r *Refresher) Refresh(userID string) {
	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()
	r.rdb.Set(ctx, presencePrefix+userID, r.gatewayID, presenceTTL)
}

func (r *Refresher) Close() error {
	return r.closer.Close()
}
