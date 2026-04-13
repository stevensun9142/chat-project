package main

import (
	"context"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/redis/go-redis/v9"
	"github.com/stevensun/chat-project/router/delivery"
	"github.com/stevensun/chat-project/router/presence"
	"github.com/stevensun/chat-project/router/store"
)

func main() {
	brokers := strings.Split(requireEnv("KAFKA_BROKERS"), ",")
	redisAddrs := strings.Split(requireEnv("REDIS_ADDRS"), ",")
	pgDSN := requireEnv("PG_DSN")
	gatewayAddrs := parseGatewayAddrs(requireEnv("GATEWAY_ADDRS"))

	// Redis for presence registry — cluster or single-node based on address count.
	var rdb redis.Cmdable
	var rdbCloser io.Closer
	if len(redisAddrs) > 1 {
		c := redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:         redisAddrs,
			ReadOnly:      true,
			RouteRandomly: true,
		})
		rdb = c
		rdbCloser = c
	} else {
		c := redis.NewClient(&redis.Options{Addr: redisAddrs[0]})
		rdb = c
		rdbCloser = c
	}
	defer rdbCloser.Close()
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("redis connect: %v", err)
	}
	log.Printf("connected to redis at %v", redisAddrs)

	// Postgres for room membership lookups.
	pg, err := store.NewPostgres(pgDSN)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pg.Close()
	log.Println("connected to postgres")

	registry := presence.NewRegistry(rdb)

	// gRPC connection pool for Gateway pods.
	pool := delivery.NewGatewayPool(gatewayAddrs)
	defer pool.Close()
	log.Printf("gateway pool configured: %v", gatewayAddrs)

	// Per-gateway batching for delivery RPCs.
	batcher := delivery.NewBatcher(pool)
	defer batcher.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var wg sync.WaitGroup

	// Presence consumer — updates user→gateway mapping in Redis.
	wg.Add(1)
	go func() {
		defer wg.Done()
		presenceConsumer := presence.NewConsumer(brokers, "router-presence", registry)
		log.Println("starting presence consumer group=router-presence topic=presence.events")
		if err := presenceConsumer.Run(ctx); err != nil {
			log.Printf("presence consumer error: %v", err)
			cancel()
		}
	}()

	// Delivery consumer — fans out messages to gateways via gRPC.
	wg.Add(1)
	go func() {
		defer wg.Done()
		deliveryConsumer := delivery.NewConsumer(brokers, "router-delivery", pg, registry, batcher)
		log.Println("starting delivery consumer group=router-delivery topic=chat.delivery")
		if err := deliveryConsumer.Run(ctx); err != nil {
			log.Printf("delivery consumer error: %v", err)
			cancel()
		}
	}()

	wg.Wait()
	log.Println("router shutdown complete")
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s environment variable is required", key)
	}
	return v
}

// parseGatewayAddrs parses "gateway-0=host:port,gateway-1=host:port" into a map.
func parseGatewayAddrs(raw string) map[string]string {
	addrs := make(map[string]string)
	for _, entry := range strings.Split(raw, ",") {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			log.Fatalf("invalid GATEWAY_ADDRS entry: %q (expected id=host:port)", entry)
		}
		addrs[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return addrs
}
