package outbox

import (
	"sync"
	"time"

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
	// preserva endpoint/autorização no envelope em memória
	m.queue = append(m.queue, e)
	return nil
}

// Len retorna contagem aproximada de itens e bytes (bytes não calculados aqui).
func (m *MemStore) Len() (int, int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.queue), 0
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

func (m *MemStore) Prune(opts PruneOptions) (int, error) {
	maxPerAgent := opts.MaxPerAgent
	maxAge := opts.MaxAge
	now := opts.effectiveNow()

	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.queue) == 0 {
		return 0, nil
	}

	removed := 0
	perAgent := make(map[string]int)
	keep := make([]entities.Envelope, 0, len(m.queue))

	for i := len(m.queue) - 1; i >= 0; i-- {
		e := m.queue[i]
		if maxAge > 0 && e.TSUnixMs > 0 {
			if now.Sub(time.UnixMilli(e.TSUnixMs)) > maxAge {
				removed++
				continue
			}
		}
		if maxPerAgent > 0 {
			if perAgent[e.AgentID] >= maxPerAgent {
				removed++
				continue
			}
			perAgent[e.AgentID]++
		}
		keep = append(keep, e)
	}

	for i, j := 0, len(keep)-1; i < j; i, j = i+1, j-1 {
		keep[i], keep[j] = keep[j], keep[i]
	}
	m.queue = keep
	return removed, nil
}
