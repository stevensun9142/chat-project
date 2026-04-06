package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/stevensun/chat-project/gateway/auth"
	"github.com/stevensun/chat-project/gateway/kafka"
	"github.com/stevensun/chat-project/gateway/ws"
)

func main() {
	port := os.Getenv("GATEWAY_PORT")
	if port == "" {
		port = "8001"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable is required")
	}

	brokersEnv := os.Getenv("KAFKA_BROKERS")
	if brokersEnv == "" {
		log.Fatal("KAFKA_BROKERS environment variable is required")
	}
	brokers := strings.Split(brokersEnv, ",")

	validator := auth.NewJWTValidator(jwtSecret)
	hub := ws.NewHub()
	producer := kafka.NewProducer(brokers)
	defer producer.Close()

	http.HandleFunc("/ws", ws.HandleUpgrade(hub, validator, producer))

	log.Printf("Gateway listening on :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
