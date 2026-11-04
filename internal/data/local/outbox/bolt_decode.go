package outbox

import (
	"encoding/json"

	"github.com/you/aiceberg_agent/internal/common/envelopejson"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func (b *BoltStore) decode(raw []byte) (entities.Envelope, error) {
	var stored storedEnvelope
	if b.rawBodies {
		var wire struct {
			Env            envelopejson.RawBody `json:"env"`
			AuthHeader     string               `json:"auth_header,omitempty"`
			IdentityHeader string               `json:"identity_header,omitempty"`
			Endpoint       string               `json:"endpoint,omitempty"`
		}
		if err := json.Unmarshal(raw, &wire); err != nil {
			return entities.Envelope{}, err
		}
		stored.Env = wire.Env.Value()
		stored.AuthHeader, stored.IdentityHeader, stored.Endpoint = wire.AuthHeader, wire.IdentityHeader, wire.Endpoint
	} else if err := json.Unmarshal(raw, &stored); err != nil {
		return entities.Envelope{}, err
	}
	env := stored.Env
	env.AuthHeader, env.IdentityHeader = stored.AuthHeader, stored.IdentityHeader
	if env.Endpoint == "" {
		env.Endpoint = stored.Endpoint
	}
	return env, nil
}
