package ports

import "github.com/you/aiceberg_agent/internal/domain/entities"

type Transport interface {
	Send(batch []entities.Envelope) error
}
