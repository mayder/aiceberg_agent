package channel

import (
	"fmt"
	"strings"
)

const (
	ContractID    = "aiceberg.agent.channel.v1"
	SchemaVersion = 1

	ModeDirect = "direct"
	ModeHub    = "hub"
	ModeRelay  = "relay"

	TypeSessionOpen = "session.open"
	TypeHeartbeat   = "heartbeat"
	TypeCommand     = "command"
	TypeAck         = "ack"
	TypeProgress    = "progress"
	TypeResult      = "result"
	TypeError       = "error"
	TypeTimeout     = "timeout"
	TypeRetry       = "retry"

	StatusAccepted  = "accepted"
	StatusRejected  = "rejected"
	StatusDuplicate = "duplicate"
	StatusRunning   = "running"
	StatusSuccess   = "success"
	StatusFailed    = "failed"
	StatusTimeout   = "timeout"
	StatusRetrying  = "retrying"

	CommandCollectNow              = "collect_now"
	CommandApplyAgentMode          = "apply_agent_mode"
	CommandRestartAgentlessWorker  = "restart_agentless_worker"
	CommandReloadConfiguration     = "reload_configuration"
	CommandClearLocalLock          = "clear_local_lock"
	CommandRequeuePendingCollect   = "requeue_pending_collect"
	CommandValidateAPIConnectivity = "validate_api_connectivity"
	CommandResyncClock             = "resync_clock"
	CommandInspectRuntimeConfig    = "inspect_runtime_config"
)

var LegacyHTTPContracts = map[string][]string{
	"ping":          {"/v1/agent/ping"},
	"bootstrap":     {"/v1/agent/bootstrap", "/v1/ingest/bootstrap"},
	"config":        {"/v1/agent/config"},
	"selfheal":      {"/v1/agent/selfheal-commands", "/v1/agent/selfheal-report"},
	"update-report": {"/v1/agent/update-report"},
	"ingest":        {"/v1/ingest", "/v1/ingest/metrics", "/v1/ingest/health", "/v1/ingest/inventory", "/v1/ingest/network_capture"},
}

type Topology struct {
	Upstream                 string
	Path                     []string
	AcceptsRelays            bool
	ConnectsDirectlyAiceberg bool
	RelayConnectsToAiceberg  bool
}

var Topologies = map[string]Topology{
	ModeDirect: {
		Upstream:                 "aiceberg",
		Path:                     []string{"agent", "aiceberg"},
		ConnectsDirectlyAiceberg: true,
		RelayConnectsToAiceberg:  false,
	},
	ModeHub: {
		Upstream:                 "aiceberg",
		Path:                     []string{"hub", "aiceberg"},
		AcceptsRelays:            true,
		ConnectsDirectlyAiceberg: true,
		RelayConnectsToAiceberg:  false,
	},
	ModeRelay: {
		Upstream:                 "hub",
		Path:                     []string{"relay", "hub", "aiceberg"},
		ConnectsDirectlyAiceberg: false,
		RelayConnectsToAiceberg:  false,
	},
}

type Envelope struct {
	ContractID    string         `json:"contract_id"`
	SchemaVersion int            `json:"schema_version"`
	MessageID     string         `json:"message_id"`
	Type          string         `json:"type"`
	TimestampUTC  string         `json:"timestamp_utc"`
	AgentID       string         `json:"agent_id"`
	HubAgentID    string         `json:"hub_agent_id,omitempty"`
	Mode          string         `json:"mode"`
	CommandID     string         `json:"command_id,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	Attempt       int            `json:"attempt,omitempty"`
	TimeoutMs     int            `json:"timeout_ms,omitempty"`
	RetryAfterMs  int            `json:"retry_after_ms,omitempty"`
	Capabilities  []string       `json:"capabilities,omitempty"`
	Payload       map[string]any `json:"payload,omitempty"`
	Progress      map[string]any `json:"progress,omitempty"`
	Result        map[string]any `json:"result,omitempty"`
	Error         map[string]any `json:"error,omitempty"`
}

func NormalizeMode(mode string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	if normalized == "direto" {
		normalized = ModeDirect
	}
	_, ok := Topologies[normalized]
	return normalized, ok
}

func TopologyForMode(mode string) (Topology, bool) {
	normalized, ok := NormalizeMode(mode)
	if !ok {
		return Topology{}, false
	}
	return Topologies[normalized], true
}

func ModeConnectsToAiceberg(mode string) bool {
	normalized, ok := NormalizeMode(mode)
	return ok && (normalized == ModeDirect || normalized == ModeHub)
}

func AllowedCommandCodes() map[string]struct{} {
	return map[string]struct{}{
		CommandCollectNow:              {},
		CommandApplyAgentMode:          {},
		CommandRestartAgentlessWorker:  {},
		CommandReloadConfiguration:     {},
		CommandClearLocalLock:          {},
		CommandRequeuePendingCollect:   {},
		CommandValidateAPIConnectivity: {},
		CommandResyncClock:             {},
		CommandInspectRuntimeConfig:    {},
	}
}

func IsAllowedCommandCode(code string) bool {
	_, ok := AllowedCommandCodes()[strings.ToLower(strings.TrimSpace(code))]
	return ok
}

func IsShellLikeCommandCode(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "shell", "exec", "command", "run", "bash", "sh", "cmd", "powershell", "script":
		return true
	default:
		return false
	}
}

func ValidateEnvelope(env Envelope) []string {
	var errors []string

	if strings.TrimSpace(env.ContractID) == "" {
		errors = append(errors, "missing required field: contract_id")
	}
	if env.ContractID != ContractID {
		errors = append(errors, "invalid channel contract")
	}
	if env.SchemaVersion == 0 {
		errors = append(errors, "missing required field: schema_version")
	}
	if env.SchemaVersion != SchemaVersion {
		errors = append(errors, "invalid channel schema version")
	}
	if strings.TrimSpace(env.MessageID) == "" {
		errors = append(errors, "missing required field: message_id")
	}
	if !isEnvelopeType(env.Type) {
		errors = append(errors, "invalid channel envelope type")
	}
	if strings.TrimSpace(env.TimestampUTC) == "" {
		errors = append(errors, "missing required field: timestamp_utc")
	}
	if strings.TrimSpace(env.AgentID) == "" {
		errors = append(errors, "missing required field: agent_id")
	}
	if _, ok := NormalizeMode(env.Mode); !ok {
		errors = append(errors, "invalid channel mode")
	}

	if requiresCommandID(env.Type) && strings.TrimSpace(env.CommandID) == "" {
		errors = append(errors, "command_id is required for command messages")
	}
	if env.Type == TypeProgress && env.Progress == nil {
		errors = append(errors, "progress is required for progress envelope")
	}
	if env.Type == TypeResult && env.Result == nil {
		errors = append(errors, "result is required for result envelope")
	}
	if env.Type == TypeError && env.Error == nil {
		errors = append(errors, "error is required for error envelope")
	}

	return errors
}

func MustTopology(mode string) Topology {
	topology, ok := TopologyForMode(mode)
	if !ok {
		panic(fmt.Sprintf("invalid channel mode %q", mode))
	}
	return topology
}

func isEnvelopeType(value string) bool {
	switch value {
	case TypeSessionOpen, TypeHeartbeat, TypeCommand, TypeAck, TypeProgress, TypeResult, TypeError, TypeTimeout, TypeRetry:
		return true
	default:
		return false
	}
}

func requiresCommandID(value string) bool {
	switch value {
	case TypeCommand, TypeAck, TypeProgress, TypeResult, TypeError, TypeTimeout, TypeRetry:
		return true
	default:
		return false
	}
}
