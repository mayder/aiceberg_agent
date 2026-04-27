package usecase

import (
	"strings"
	"sync"
	"time"
)

type CommandIdempotency struct {
	mu       sync.Mutex
	seen     map[string]time.Time
	maxAge   time.Duration
	maxItems int
}

func NewCommandIdempotency(maxAge time.Duration, maxItems int) *CommandIdempotency {
	if maxAge <= 0 {
		maxAge = time.Hour
	}
	if maxItems <= 0 {
		maxItems = 512
	}
	return &CommandIdempotency{
		seen:     make(map[string]time.Time),
		maxAge:   maxAge,
		maxItems: maxItems,
	}
}

func (d *CommandIdempotency) First(commandID string) bool {
	commandID = strings.TrimSpace(commandID)
	if commandID == "" {
		return false
	}
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pruneLocked(now)
	if _, exists := d.seen[commandID]; exists {
		return false
	}
	d.seen[commandID] = now
	return true
}

func (d *CommandIdempotency) pruneLocked(now time.Time) {
	for commandID, seenAt := range d.seen {
		if now.Sub(seenAt) > d.maxAge {
			delete(d.seen, commandID)
		}
	}
	if len(d.seen) <= d.maxItems {
		return
	}
	var oldestID string
	var oldestAt time.Time
	for commandID, seenAt := range d.seen {
		if oldestID == "" || seenAt.Before(oldestAt) {
			oldestID = commandID
			oldestAt = seenAt
		}
	}
	if oldestID != "" {
		delete(d.seen, oldestID)
	}
}
