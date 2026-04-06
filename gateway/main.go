package main

import (
	"log"
	"net/http"
	"os"

	"github.com/stevensun/chat-project/gateway/auth"
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

	validator := auth.NewJWTValidator(jwtSecret)
	hub := ws.NewHub()

	http.HandleFunc("/ws", ws.HandleUpgrade(hub, validator))

	log.Printf("Gateway listening on :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
