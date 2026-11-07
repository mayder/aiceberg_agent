package repositories

import (
	"errors"
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

func (r *agentlessOutboxRepo) ReadDeliveryBatch(n int, ordinal uint64, excluded map[string]bool) ([]entities.AgentlessObservation, error) {
	if reader, ok := r.store.(interface {
		PeekDelivery(int, uint64, map[string]bool) ([]entities.AgentlessObservation, error)
	}); ok {
		return reader.PeekDelivery(n, ordinal, excluded)
	}
	return nil, errors.New("agentless priority selection unsupported by store")
}
