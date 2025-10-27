package repositories

import (
	"github.com/you/aiceberg_agent/internal/data/local"
	"github.com/you/aiceberg_agent/internal/data/remote"
)

// TelemetryRepository orquestra local (outbox) + remoto (ingest).
// Ele NÃO conhece detalhes de bbolt ou net/http; apenas as interfaces.
// Use este repo em casos de uso ou como adaptador p/ domain ports.
type TelemetryRepository interface {
	Save(topic string, payload []byte) error
	Flush(endpoint string, headers map[string]string, maxBytes int) (flushed int, err error)
}

// telemetryRepoImpl é a implementação padrão do repositório.
type telemetryRepoImpl struct {
	outbox local.OutboxDataSource
	ingest remote.IngestClient
}

func NewTelemetryRepository(outbox local.OutboxDataSource, ingest remote.IngestClient) TelemetryRepository {
	return &telemetryRepoImpl{outbox: outbox, ingest: ingest}
}

func (r *telemetryRepoImpl) Save(topic string, payload []byte) error {
	return r.outbox.Append(topic, payload)
}

func (r *telemetryRepoImpl) Flush(endpoint string, headers map[string]string, maxBytes int) (int, error) {
	keys, batch, err := r.outbox.ReadBatch(maxBytes)
	if err != nil || len(batch) == 0 {
		return 0, err
	}
	// empacota como JSON array simples
	body := make([]byte, 0, 2+len(batch)*64)
	body = append(body, '[')
	for i, p := range batch {
		if i > 0 {
			body = append(body, ',')
		}
		body = append(body, p...)
	}
	body = append(body, ']')

	status, _, err := r.ingest.SendBatch(endpoint, body, headers)
	if err != nil || status < 200 || status >= 300 {
		return 0, err
	}
	if err := r.outbox.Commit(keys); err != nil {
		return 0, err
	}
	return len(batch), nil
}
