package repositories

import (
	"encoding/json"
	"fmt"

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
func (a *TransportAdapter) SendWithAuth(batch []entities.Envelope, authHeader string, endpoint string) ([]byte, error) {
	impl, ok := a.repo.(*telemetryRepoImpl)
	if !ok {
		return nil, nil
	}

	payload, err := json.Marshal(batch)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{"Content-Type": "application/json"}
	if authHeader != "" {
		headers["Authorization"] = authHeader
	}
	if endpoint == "" {
		endpoint = "/v1/ingest"
	}
	status, resp, err := impl.ingest.SendBatch(endpoint, payload, headers)
	if err != nil {
		return resp, err
	}
	if status < 200 || status >= 300 {
		return resp, fmt.Errorf("http %d", status)
	}
	return resp, nil
}

// Garante conformidade
var _ ports.Transport = (*TransportAdapter)(nil)
