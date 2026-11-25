package ports

import "github.com/you/aiceberg_agent/internal/domain/entities"

// AgentlessOutboxRepo é o contrato de armazenamento da fila agentless.
// Requisitos:
// - Append não deve mutar a observação recebida.
// - ReadBatch retorna até n itens, sem removê-los.
// - Ack deve ser idempotente e ignorar IDs desconhecidos.
type AgentlessOutboxRepo interface {
	Append(obs entities.AgentlessObservation) error
	ReadBatch(n int) ([]entities.AgentlessObservation, error)
	Ack(ids []string) error
	Len() (items int, bytes int64)
}

// AgentlessOutboxAdmission bounds how many minimum-size observations could
// fit. Append still validates the actual serialized size.
type AgentlessOutboxAdmission interface {
	AdmissionState() (items int, maxNewItems int64, err error)
}

// AgentlessDeliveryReader optionally provides stream-safe, fair selection.
// Excluded streams stay pending; reading never acknowledges or removes data.
type AgentlessDeliveryReader interface {
	ReadDeliveryBatch(n int, ordinal uint64, excluded map[string]bool) ([]entities.AgentlessObservation, error)
}
