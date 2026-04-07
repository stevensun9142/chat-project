package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/stevensun/chat-project/gateway/auth"
	"github.com/stevensun/chat-project/gateway/id"
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

	gatewayID := os.Getenv("GATEWAY_ID")
	if gatewayID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			log.Fatalf("failed to get hostname for GATEWAY_ID: %v", err)
		}
		gatewayID = hostname
	}

	validator := auth.NewJWTValidator(jwtSecret)
	hub := ws.NewHub()
	producer := kafka.NewProducer(brokers, gatewayID)
	defer producer.Close()
	idgen := id.NewGenerator(gatewayID)

	http.HandleFunc("/ws", ws.HandleUpgrade(hub, validator, producer, idgen))

	log.Printf("Gateway listening on :%s (id=%s, machine=%d)", port, gatewayID, idgen.MachineID())
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
