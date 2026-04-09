package delivery

import (
	"context"
	"log"
	"sync"
	"time"

	pb "github.com/stevensun/chat-project/proto"
)

const (
	flushCount   = 64
	flushTimeout = 5 * time.Millisecond
	maxBuffer    = 1024
)

// gatewayBuffer accumulates messages for one gateway and flushes on count or timer.
type gatewayBuffer struct {
	mu      sync.Mutex
	msgs    []*pb.DeliverMessage
	timer   *time.Timer
	batcher *Batcher
	gateway string
}

// appendOrDrop adds a message. Returns true if added, false if buffer full (dropped).
func (gb *gatewayBuffer) appendOrDrop(msg *pb.DeliverMessage) bool {
	gb.mu.Lock()
	defer gb.mu.Unlock()

	if len(gb.msgs) >= maxBuffer {
		return false
	}

	first := len(gb.msgs) == 0
	gb.msgs = append(gb.msgs, msg)

	if len(gb.msgs) >= flushCount {
		gb.stopTimerLocked()
		gb.flushLocked()
		return true
	}

	if first {
		gb.timer = time.AfterFunc(flushTimeout, gb.timerFlush)
	}
	return true
}

// timerFlush is called by time.AfterFunc when the deadline fires.
func (gb *gatewayBuffer) timerFlush() {
	gb.mu.Lock()
	defer gb.mu.Unlock()
	gb.flushLocked()
}

// flushLocked sends the buffered messages via gRPC. Caller must hold gb.mu.
func (gb *gatewayBuffer) flushLocked() {
	if len(gb.msgs) == 0 {
		return
	}

	batch := gb.msgs
	gb.msgs = nil
	gb.stopTimerLocked()

	// Release the lock during the gRPC call.
	gb.mu.Unlock()
	gb.batcher.send(gb.gateway, batch)
	gb.mu.Lock()
}

func (gb *gatewayBuffer) stopTimerLocked() {
	if gb.timer != nil {
		gb.timer.Stop()
		gb.timer = nil
	}
}

// drainLocked flushes remaining messages and prevents future timer fires.
func (gb *gatewayBuffer) drainLocked() {
	gb.stopTimerLocked()
	if len(gb.msgs) == 0 {
		return
	}
	batch := gb.msgs
	gb.msgs = nil

	gb.mu.Unlock()
	gb.batcher.send(gb.gateway, batch)
	gb.mu.Lock()
}

// Batcher groups DeliverMessages by gateway and flushes via gRPC in batches.
// All methods must be called from a single goroutine (the delivery consumer).
type Batcher struct {
	pool    *GatewayPool
	buffers map[string]*gatewayBuffer
}

func NewBatcher(pool *GatewayPool) *Batcher {
	return &Batcher{
		pool:    pool,
		buffers: make(map[string]*gatewayBuffer),
	}
}

// Add queues a message for the given gateway. Drops with a log if the buffer is full.
func (b *Batcher) Add(gatewayID string, msg *pb.DeliverMessage) {
	gb, ok := b.buffers[gatewayID]
	if !ok {
		gb = &gatewayBuffer{
			batcher: b,
			gateway: gatewayID,
		}
		b.buffers[gatewayID] = gb
	}

	if !gb.appendOrDrop(msg) {
		log.Printf("batcher: dropped message for gateway=%s: buffer full (%d)", gatewayID, maxBuffer)
	}
}

// Close flushes all remaining buffers. Safe to call once.
func (b *Batcher) Close() {
	for _, gb := range b.buffers {
		gb.mu.Lock()
		gb.drainLocked()
		gb.mu.Unlock()
	}
}

// send performs the gRPC Deliver call. Called without any locks held.
func (b *Batcher) send(gatewayID string, msgs []*pb.DeliverMessage) {
	client, err := b.pool.Client(gatewayID)
	if err != nil {
		log.Printf("batcher: grpc client gateway=%s: %v", gatewayID, err)
		return
	}

	req := &pb.DeliverRequest{Messages: msgs}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	resp, err := client.Deliver(ctx, req)
	cancel()
	if err != nil {
		log.Printf("batcher: grpc deliver gateway=%s batch=%d: %v", gatewayID, len(msgs), err)
		return
	}

	log.Printf("batcher: gateway=%s batch=%d delivered=%d", gatewayID, len(msgs), resp.Delivered)
}
