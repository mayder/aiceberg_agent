package repositories

import (
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

// Store é a porta local usada pelo repositório.
type Store interface {
	Push(e entities.Envelope) error
	Peek(n int) ([]entities.Envelope, error)
	Delete(ids []string) error
}

type outboxRepo struct{ store Store }

func NewOutboxRepository(s Store) ports.OutboxRepo { return &outboxRepo{store: s} }

func (r *outboxRepo) Append(env entities.Envelope) error { return r.store.Push(env) }

func (r *outboxRepo) ReadBatch(n int) ([]entities.Envelope, error) { return r.store.Peek(n) }

func (r *outboxRepo) Ack(ids []string) error { return r.store.Delete(ids) }
