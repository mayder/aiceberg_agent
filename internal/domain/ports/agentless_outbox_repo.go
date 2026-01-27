package ports

import "github.com/you/aiceberg_agent/internal/domain/entities"

type AgentlessOutboxRepo interface {
	Append(obs entities.AgentlessObservation) error
	ReadBatch(n int) ([]entities.AgentlessObservation, error)
	Ack(ids []string) error
	Len() (items int, bytes int64)
}
