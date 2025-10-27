package ports

import "github.com/you/aiceberg_agent/internal/domain/entities"

type OutboxRepo interface {
	Append(env entities.Envelope) error
	ReadBatch(n int) ([]entities.Envelope, error)
	Ack(ids []string) error
}
