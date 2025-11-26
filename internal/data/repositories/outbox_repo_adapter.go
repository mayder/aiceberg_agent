package repositories

import (
	"encoding/json"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

// OutboxRepoAdapter implementa ports.OutboxRepo usando TelemetryRepository + Outbox local.
type OutboxRepoAdapter struct {
	repo TelemetryRepository
}

func NewOutboxRepoAdapter(repo TelemetryRepository) *OutboxRepoAdapter {
	return &OutboxRepoAdapter{repo: repo}
}

// Append adiciona um Envelope serializado no outbox.
func (a *OutboxRepoAdapter) Append(env entities.Envelope) error {
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return a.repo.Save("outbox", data)
}

// ReadBatch lê até n envelopes do outbox.
func (a *OutboxRepoAdapter) ReadBatch(n int) ([]entities.Envelope, error) {
	impl, ok := a.repo.(*telemetryRepoImpl)
	if !ok {
		return nil, nil
	}
	keys, payloads, err := impl.outbox.ReadBatch(n)
	if err != nil {
		return nil, err
	}

	out := make([]entities.Envelope, 0, len(payloads))
	for i := range payloads {
		var e entities.Envelope
		if err := json.Unmarshal(payloads[i], &e); err == nil {
			out = append(out, e)
		}
	}
	// opcional: guardar keys em impl para Ack
	_ = keys
	return out, nil
}

// Ack confirma processamento e remove itens lidos.
func (a *OutboxRepoAdapter) Ack(ids []string) error {
	impl, ok := a.repo.(*telemetryRepoImpl)
	if !ok {
		return nil
	}
	// aqui converter ids → keys, se necessário.
	// versão mínima:
	return impl.outbox.Commit(nil)
}

// Len retorna 0; TelemetryRepository não expõe contagem no momento.
func (a *OutboxRepoAdapter) Len() (int, int64) { return 0, 0 }

// Garante conformidade.
var _ ports.OutboxRepo = (*OutboxRepoAdapter)(nil)
