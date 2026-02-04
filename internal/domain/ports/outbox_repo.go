package ports

import "github.com/you/aiceberg_agent/internal/domain/entities"

// OutboxRepo é o contrato de armazenamento da fila principal de envelopes.
// Requisitos:
// - Append não deve mutar o envelope recebido.
// - ReadBatch retorna até n itens, sem removê-los.
// - Ack deve ser idempotente e ignorar IDs desconhecidos.
type OutboxRepo interface {
	Append(env entities.Envelope) error
	ReadBatch(n int) ([]entities.Envelope, error)
	Ack(ids []string) error
	Len() (items int, bytes int64)
}
