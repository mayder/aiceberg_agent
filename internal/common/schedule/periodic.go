package schedule

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"
)

func Identity(installationID string, clientID, agentID int, hostname string) string {
	if value := strings.TrimSpace(installationID); value != "" {
		return value
	}
	return fmt.Sprintf("%d:%d:%s", clientID, agentID, strings.TrimSpace(hostname))
}

func InitialDelay(identity, routine string, interval time.Duration) time.Duration {
	if interval <= 0 {
		return 0
	}
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(strings.TrimSpace(identity)))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(strings.TrimSpace(routine)))
	return time.Duration(hash.Sum64() % uint64(interval))
}

// PeriodicTimer distributes only the first execution. Reset preserves the exact configured interval.
type PeriodicTimer struct {
	timer    *time.Timer
	interval time.Duration
}

func NewPeriodicTimer(identity, routine string, interval time.Duration) *PeriodicTimer {
	return &PeriodicTimer{timer: time.NewTimer(InitialDelay(identity, routine, interval)), interval: interval}
}

func (p *PeriodicTimer) C() <-chan time.Time {
	if p == nil {
		return nil
	}
	return p.timer.C
}

func (p *PeriodicTimer) Reset() {
	if p != nil {
		p.timer.Reset(p.interval)
	}
}

func (p *PeriodicTimer) Stop() {
	if p != nil {
		p.timer.Stop()
	}
}
