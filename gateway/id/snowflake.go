package id

import (
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"time"
)

const (
	// Custom epoch: 2026-01-01T00:00:00Z
	epoch int64 = 1767225600000

	machineBits  = 10
	sequenceBits = 12

	maxMachineID = (1 << machineBits) - 1  // 1023
	maxSequence  = (1 << sequenceBits) - 1 // 4095

	machineShift   = sequenceBits               // 12
	timestampShift = machineBits + sequenceBits // 22
)

// Generator produces snowflake-style 64-bit IDs.
// Layout: 1 bit unused | 41 bits timestamp (ms) | 10 bits machine_id | 12 bits sequence
type Generator struct {
	mu            sync.Mutex
	machineID     int64
	lastTimestamp int64
	sequence      int64
}

// NewGenerator creates a Generator with a machine ID extracted from the hostname.
// Expects hostnames like "gateway-0", "gateway-2" — parses the trailing integer.
// Falls back to 0 if no trailing number is found.
func NewGenerator(hostname string) *Generator {
	return &Generator{
		machineID: parseMachineID(hostname),
	}
}

// NextID returns a unique snowflake ID. Thread-safe.
func (g *Generator) NextID() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now().UnixMilli() - epoch

	if now == g.lastTimestamp {
		g.sequence = (g.sequence + 1) & maxSequence
		if g.sequence == 0 {
			// Sequence exhausted for this millisecond — spin until next ms
			for now <= g.lastTimestamp {
				now = time.Now().UnixMilli() - epoch
			}
		}
	} else {
		g.sequence = 0
	}

	g.lastTimestamp = now

	return (now << timestampShift) | (g.machineID << machineShift) | g.sequence
}

var trailingNumber = regexp.MustCompile(`(\d+)$`)

func parseMachineID(hostname string) int64 {
	match := trailingNumber.FindString(hostname)
	if match == "" {
		return 0
	}
	n, err := strconv.ParseInt(match, 10, 64)
	if err != nil || n < 0 || n > maxMachineID {
		return 0
	}
	return n
}

// MachineID returns the machine ID for logging/debugging.
func (g *Generator) MachineID() int64 {
	return g.machineID
}

// ExtractTime returns the timestamp embedded in a snowflake ID as a time.Time.
func ExtractTime(id int64) time.Time {
	ms := (id >> timestampShift) + epoch
	return time.UnixMilli(ms)
}

// FormatID returns a human-readable breakdown of a snowflake ID for debugging.
func FormatID(id int64) string {
	ts := id >> timestampShift
	machine := (id >> machineShift) & maxMachineID
	seq := id & maxSequence
	return fmt.Sprintf("id=%d ts=%d machine=%d seq=%d", id, ts, machine, seq)
}
