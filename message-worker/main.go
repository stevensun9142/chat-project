package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/stevensun/chat-project/message-worker/consumer"
	"github.com/stevensun/chat-project/message-worker/store"
)

func main() {
	brokersEnv := os.Getenv("KAFKA_BROKERS")
	if brokersEnv == "" {
		log.Fatal("KAFKA_BROKERS environment variable is required")
	}
	brokers := strings.Split(brokersEnv, ",")

	groupID := os.Getenv("KAFKA_GROUP_ID")
	if groupID == "" {
		groupID = "message-worker"
	}

	cassHosts := strings.Split(getEnv("CASS_HOSTS", "localhost"), ",")
	cassPort := getEnv("CASS_PORT", "9042")
	cassKeyspace := getEnv("CASS_KEYSPACE", "chat")

	pgDSN := getEnv("PG_DSN", "postgres://chat:chat_secret@localhost:5432/chat_db?sslmode=disable")

	cass, err := store.NewCassandra(cassHosts, cassPort, cassKeyspace)
	if err != nil {
		log.Fatalf("cassandra connect: %v", err)
	}
	defer cass.Close()
	log.Printf("connected to cassandra keyspace=%s", cassKeyspace)

	pg, err := store.NewPostgres(pgDSN)
	if err != nil {
		log.Fatalf("postgres connect: %v", err)
	}
	defer pg.Close()
	log.Println("connected to postgres")

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	c := consumer.New(brokers, groupID, cass, pg)
	log.Printf("starting consumer group=%s topic=chat.messages", groupID)

	if err := c.Run(ctx); err != nil {
		log.Fatalf("consumer error: %v", err)
	}

	log.Println("worker shutdown complete")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
