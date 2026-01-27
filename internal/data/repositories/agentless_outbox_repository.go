package repositories

import (
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

type AgentlessStore interface {
	Push(obs entities.AgentlessObservation) error
	Peek(n int) ([]entities.AgentlessObservation, error)
	Delete(ids []string) error
	Len() (items int, bytes int64)
}

type agentlessOutboxRepo struct{ store AgentlessStore }

func NewAgentlessOutboxRepository(s AgentlessStore) ports.AgentlessOutboxRepo {
	return &agentlessOutboxRepo{store: s}
}

func (r *agentlessOutboxRepo) Append(obs entities.AgentlessObservation) error {
	return r.store.Push(obs)
}

func (r *agentlessOutboxRepo) ReadBatch(n int) ([]entities.AgentlessObservation, error) {
	return r.store.Peek(n)
}

func (r *agentlessOutboxRepo) Ack(ids []string) error {
	return r.store.Delete(ids)
}

func (r *agentlessOutboxRepo) Len() (int, int64) { return r.store.Len() }
