package outbox

import (
	"sync"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

// MemStore: implementação simples em memória.
type MemStore struct {
	mu    sync.Mutex
	queue []entities.Envelope
}

func NewMemStore() *MemStore { return &MemStore{} }

func (m *MemStore) Push(e entities.Envelope) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queue = append(m.queue, e)
	return nil
}

func (m *MemStore) Peek(n int) ([]entities.Envelope, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n > len(m.queue) {
		n = len(m.queue)
	}
	cp := make([]entities.Envelope, n)
	copy(cp, m.queue[:n])
	return cp, nil
}

func (m *MemStore) Delete(ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	keep := m.queue[:0]
outer:
	for _, e := range m.queue {
		for _, id := range ids {
			if e.ID == id {
				continue outer
			}
		}
		keep = append(keep, e)
	}
	m.queue = keep
	return nil
}
