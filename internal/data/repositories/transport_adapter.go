package repositories

import (
	"encoding/json"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

// TransportAdapter implementa ports.Transport usando o IngestClient (remote).
type TransportAdapter struct {
	repo TelemetryRepository
}

func NewTransportAdapter(repo TelemetryRepository) *TransportAdapter {
	return &TransportAdapter{repo: repo}
}

// Implementa ports.Transport
func (a *TransportAdapter) SendWithAuth(batch []entities.Envelope, authHeader string) error {
	impl, ok := a.repo.(*telemetryRepoImpl)
	if !ok {
		return nil
	}

	payload, err := json.Marshal(batch)
	if err != nil {
		return err
	}

	headers := map[string]string{"Content-Type": "application/json"}
	if authHeader != "" {
		headers["Authorization"] = authHeader
	}
	_, _, err = impl.ingest.SendBatch("/v1/ingest", payload, headers)
	return err
}

// Garante conformidade
var _ ports.Transport = (*TransportAdapter)(nil)
