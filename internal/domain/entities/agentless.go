package entities

import (
	"bytes"
	"encoding/json"
	"time"
)

type AgentlessConfig map[string]any

func (c *AgentlessConfig) UnmarshalJSON(b []byte) error {
	raw := bytes.TrimSpace(b)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		*c = nil
		return nil
	}
	switch raw[0] {
	case '{':
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return err
		}
		*c = AgentlessConfig(m)
		return nil
	case '[':
		// Backend pode enviar lista vazia. Para o agente, tratamos como config vazia.
		var arr []any
		if err := json.Unmarshal(raw, &arr); err != nil {
			return err
		}
		*c = AgentlessConfig{}
		return nil
	default:
		// fallback: tenta mapa
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return err
		}
		*c = AgentlessConfig(m)
		return nil
	}
}

type AgentlessEndpoint struct {
	ID       int    `json:"id"`
	Tipo     string `json:"tipo"`
	Endereco string `json:"endereco"`
	Porta    *int   `json:"porta,omitempty"`
	TLSSNI   string `json:"tls_sni,omitempty"`
}

type AgentlessSnmpProfile struct {
	BindingID      int    `json:"binding_id"`
	ProfileID      int    `json:"profile_id"`
	Version        string `json:"version"`
	Community      string `json:"community,omitempty"`
	V3User         string `json:"v3_user,omitempty"`
	V3AuthProtocol string `json:"v3_auth_protocol,omitempty"`
	V3AuthPassword string `json:"v3_auth_password,omitempty"`
	V3PrivProtocol string `json:"v3_priv_protocol,omitempty"`
	V3PrivPassword string `json:"v3_priv_password,omitempty"`
	ContextName    string `json:"context_name,omitempty"`
	Port           int    `json:"port,omitempty"`
	TimeoutMs      int    `json:"timeout_ms,omitempty"`
	TimeBudgetMs   int    `json:"time_budget_ms,omitempty"`
	Retries        int    `json:"retries,omitempty"`
}

type AgentlessJob struct {
	CheckID        int                   `json:"check_id"`
	AtivoID        int                   `json:"ativo_id"`
	ClienteID      int                   `json:"cliente_id"`
	Tipo           string                `json:"tipo"`
	Nome           string                `json:"nome,omitempty"`
	IntervalSec    int                   `json:"interval_sec"`
	TimeoutMs      int                   `json:"timeout_ms"`
	CollectionKind string                `json:"snmp_collection_kind,omitempty"`
	Retries        int                   `json:"retries"`
	FailThreshold  int                   `json:"fail_threshold"`
	SuccessThresh  int                   `json:"success_threshold"`
	Config         AgentlessConfig       `json:"config"`
	Endpoint       *AgentlessEndpoint    `json:"endpoint,omitempty"`
	SNMP           *AgentlessSnmpProfile `json:"snmp,omitempty"`
}

type AgentlessObservation struct {
	ID               string         `json:"id"`
	CheckID          int            `json:"check_id"`
	Status           string         `json:"status"`
	LatencyMs        int            `json:"latency_ms,omitempty"`
	Code             string         `json:"code,omitempty"`
	Message          string         `json:"message,omitempty"`
	Payload          map[string]any `json:"payload_json,omitempty"`
	ObservedAt       time.Time      `json:"observed_at"`
	CollectionKind   string         `json:"snmp_collection_kind,omitempty"`
	SegmentID        string         `json:"segment_id,omitempty"`
	SegmentSeq       int            `json:"segment_seq,omitempty"`
	IsPartial        bool           `json:"is_partial,omitempty"`
	IsFinal          bool           `json:"is_final,omitempty"`
	SegmentStartedAt *time.Time     `json:"segment_started_at,omitempty"`
	DedupeKey        string         `json:"dedupe_key,omitempty"`
	EndpointID       *int           `json:"endpoint_id,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
}
