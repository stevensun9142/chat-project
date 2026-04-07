package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/redis/go-redis/v9"
	"github.com/stevensun/chat-project/router/presence"
)

func main() {
	brokers := strings.Split(requireEnv("KAFKA_BROKERS"), ",")
	redisAddr := requireEnv("REDIS_ADDR")

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("redis connect: %v", err)
	}
	log.Printf("connected to redis at %s", redisAddr)

	registry := presence.NewRegistry(rdb)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	presenceConsumer := presence.NewConsumer(brokers, "router-presence", registry)
	log.Println("starting presence consumer group=router-presence topic=presence.events")

	if err := presenceConsumer.Run(ctx); err != nil {
		log.Fatalf("presence consumer error: %v", err)
	}

	log.Println("router shutdown complete")
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s environment variable is required", key)
	}
	return v
}
