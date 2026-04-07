package grpc

import (
	"context"
	"encoding/json"
	"log"

	"github.com/stevensun/chat-project/gateway/ws"
	pb "github.com/stevensun/chat-project/proto"
)

// Server implements the Delivery gRPC service.
type Server struct {
	pb.UnimplementedDeliveryServer
	hub *ws.Hub
}

func NewServer(hub *ws.Hub) *Server {
	return &Server{hub: hub}
}

func (s *Server) Deliver(_ context.Context, req *pb.DeliverRequest) (*pb.DeliverResponse, error) {
	var delivered int32

	for _, msg := range req.Messages {
		payload, err := json.Marshal(ws.ServerMessage{
			Type:       "new_message",
			MessageID:  msg.MessageId,
			RoomID:     msg.RoomId,
			SenderID:   msg.SenderId,
			SenderName: msg.SenderName,
			Content:    msg.Content,
			CreatedAt:  msg.CreatedAt,
		})
		if err != nil {
			log.Printf("grpc deliver: marshal error: %v", err)
			continue
		}

		for _, userID := range msg.UserIds {
			client, ok := s.hub.Get(userID)
			if !ok {
				continue
			}
			select {
			case client.Send() <- payload:
				delivered++
			default:
				log.Printf("grpc deliver: send buffer full user=%s", userID)
			}
		}
	}

	return &pb.DeliverResponse{Delivered: delivered}, nil
}
