package delivery

import (
	"fmt"
	"sync"

	pb "github.com/stevensun/chat-project/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// GatewayPool manages gRPC connections to Gateway pods.
// Gateway IDs map to addresses via a provided lookup function.
type GatewayPool struct {
	mu    sync.RWMutex
	conns map[string]*grpc.ClientConn
	addrs map[string]string // gateway_id -> "host:port"
}

// NewGatewayPool creates a pool with a static mapping of gateway IDs to gRPC addresses.
func NewGatewayPool(addrs map[string]string) *GatewayPool {
	return &GatewayPool{
		conns: make(map[string]*grpc.ClientConn),
		addrs: addrs,
	}
}

// Client returns a gRPC Delivery client for the given gateway ID.
// Connections are created on first use and reused afterwards.
func (p *GatewayPool) Client(gatewayID string) (pb.DeliveryClient, error) {
	p.mu.RLock()
	conn, ok := p.conns[gatewayID]
	p.mu.RUnlock()
	if ok {
		return pb.NewDeliveryClient(conn), nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock.
	if conn, ok := p.conns[gatewayID]; ok {
		return pb.NewDeliveryClient(conn), nil
	}

	addr, ok := p.addrs[gatewayID]
	if !ok {
		return nil, fmt.Errorf("unknown gateway: %s", gatewayID)
	}

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial gateway %s at %s: %w", gatewayID, addr, err)
	}

	p.conns[gatewayID] = conn
	return pb.NewDeliveryClient(conn), nil
}

// Close closes all cached connections.
func (p *GatewayPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, conn := range p.conns {
		conn.Close()
		delete(p.conns, id)
	}
}
