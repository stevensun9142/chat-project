package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/stevensun/chat-project/gateway/auth"
	delivery "github.com/stevensun/chat-project/gateway/grpc"
	"github.com/stevensun/chat-project/gateway/id"
	"github.com/stevensun/chat-project/gateway/kafka"
	"github.com/stevensun/chat-project/gateway/presence"
	"github.com/stevensun/chat-project/gateway/ratelimit"
	"github.com/stevensun/chat-project/gateway/ws"
	"google.golang.org/grpc"

	pb "github.com/stevensun/chat-project/proto"
)

func main() {
	port := requireEnv("GATEWAY_PORT")
	jwtSecret := requireEnv("JWT_SECRET")
	brokers := strings.Split(requireEnv("KAFKA_BROKERS"), ",")
	grpcPort := requireEnv("GRPC_PORT")
	redisRatelimitAddr := requireEnv("REDIS_RATELIMIT_ADDR")
	redisPresenceAddr := requireEnv("REDIS_PRESENCE_ADDR")

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

	limiter, err := ratelimit.NewLimiter(redisRatelimitAddr, 20, 2.0)
	if err != nil {
		log.Fatalf("rate limiter init: %v", err)
	}
	defer limiter.Close()

	refresher, err := presence.NewRefresher(redisPresenceAddr, gatewayID)
	if err != nil {
		log.Fatalf("presence refresher init: %v", err)
	}
	defer refresher.Close()

	http.HandleFunc("/ws", ws.HandleUpgrade(hub, validator, producer, idgen, limiter, refresher))

	// Start gRPC server for receiving routed messages from Router.
	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		log.Fatalf("gRPC listen failed: %v", err)
	}
	gs := grpc.NewServer()
	pb.RegisterDeliveryServer(gs, delivery.NewServer(hub))
	go func() {
		log.Printf("gRPC listening on :%s", grpcPort)
		if err := gs.Serve(lis); err != nil {
			log.Fatalf("gRPC serve failed: %v", err)
		}
	}()

	log.Printf("Gateway listening on :%s (id=%s, machine=%d)", port, gatewayID, idgen.MachineID())
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s environment variable is required", key)
	}
	return v
}
