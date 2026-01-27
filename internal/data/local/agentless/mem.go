package agentless

import (
	"sync"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type MemStore struct {
	mu    sync.Mutex
	queue []entities.AgentlessObservation
}

func NewMemStore() *MemStore { return &MemStore{} }

func (m *MemStore) Push(o entities.AgentlessObservation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queue = append(m.queue, o)
	return nil
}

func (m *MemStore) Len() (int, int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.queue), 0
}

func (m *MemStore) Peek(n int) ([]entities.AgentlessObservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n > len(m.queue) {
		n = len(m.queue)
	}
	cp := make([]entities.AgentlessObservation, n)
	copy(cp, m.queue[:n])
	return cp, nil
}

func (m *MemStore) Delete(ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	keep := m.queue[:0]
outer:
	for _, o := range m.queue {
		for _, id := range ids {
			if o.ID == id {
				continue outer
			}
		}
		keep = append(keep, o)
	}
	m.queue = keep
	return nil
}

func (m *MemStore) Prune(maxAge time.Duration) int {
	if maxAge <= 0 {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	removed := 0
	now := time.Now()
	keep := make([]entities.AgentlessObservation, 0, len(m.queue))
	for _, o := range m.queue {
		if now.Sub(o.CreatedAt) > maxAge {
			removed++
			continue
		}
		keep = append(keep, o)
	}
	m.queue = keep
	return removed
}
