package envelopejson

import (
	"encoding/json"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

// RawBody decodes routing fields while keeping the validated JSON body opaque.
// RawMessage owns its bytes, so no database transaction memory escapes.
type RawBody struct {
	*entities.Envelope
	Body json.RawMessage `json:"body"`
}

func (w RawBody) Value() entities.Envelope {
	var e entities.Envelope
	if w.Envelope != nil {
		e = *w.Envelope
	}
	if len(w.Body) > 0 {
		e.Body = w.Body
	}
	return e
}

func DecodeBatch(data []byte) ([]entities.Envelope, error) {
	var wire []RawBody
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, err
	}
	batch := make([]entities.Envelope, len(wire))
	for i, w := range wire {
		batch[i] = w.Value()
	}
	return batch, nil
}
