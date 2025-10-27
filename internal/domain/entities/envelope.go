package entities

type Envelope struct {
	ID            string            `json:"envelope_id"` // era EnvelopeID
	TenantID      string            `json:"tenant_id,omitempty"`
	AgentID       string            `json:"agent_id"`
	SchemaVersion uint32            `json:"schema_version"`
	Kind          string            `json:"kind"` // metric|event|detection|heartbeat
	Sub           string            `json:"sub,omitempty"`
	TSUnixMs      int64             `json:"ts_unix_ms"`
	Meta          map[string]string `json:"meta,omitempty"`
	Body          any               `json:"body"`
}
