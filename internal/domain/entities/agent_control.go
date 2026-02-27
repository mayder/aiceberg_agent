package entities

import "time"

type WorkerErrorEvent struct {
	Source         string         `json:"source,omitempty"`
	ErrorType      string         `json:"error_type"`
	Severity       string         `json:"severity,omitempty"`
	RecoveryStatus string         `json:"recovery_status,omitempty"`
	Fingerprint    string         `json:"fingerprint,omitempty"`
	Summary        string         `json:"summary,omitempty"`
	Stack          string         `json:"stack,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	OccurredAt     string         `json:"occurred_at,omitempty"`
}

type SelfHealCommand struct {
	CommandID     string         `json:"command_id"`
	Code          string         `json:"code"`
	Mode          string         `json:"mode,omitempty"`
	Payload       map[string]any `json:"payload,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	TriggerRule   string         `json:"trigger_rule,omitempty"`
	RequestedAt   string         `json:"requested_at,omitempty"`
}

type SelfHealReport struct {
	CommandID     string         `json:"command_id"`
	Status        string         `json:"status"`
	Message       string         `json:"message,omitempty"`
	Evidence      map[string]any `json:"evidence,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	ReportedAtMs  int64          `json:"reported_at_ms,omitempty"`
}

func NewSelfHealReport(commandID, status string) SelfHealReport {
	return SelfHealReport{
		CommandID:    commandID,
		Status:       status,
		ReportedAtMs: time.Now().UnixMilli(),
	}
}
