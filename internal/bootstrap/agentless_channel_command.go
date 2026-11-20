package app

import (
	"fmt"
	"github.com/you/aiceberg_agent/internal/domain/channel"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
	"strings"
)

func channelEnvelopeAgentlessCommand(env channel.Envelope) usecase.AgentlessCommandRequest {
	req := usecase.AgentlessCommandRequest{
		CommandID:     strings.TrimSpace(env.CommandID),
		CorrelationID: strings.TrimSpace(env.CorrelationID),
	}
	if env.Payload == nil {
		return req
	}
	if commandID := strings.TrimSpace(fmt.Sprint(env.Payload["command_id"])); commandID != "" && commandID != "<nil>" {
		req.CommandID = commandID
	}
	if correlationID := strings.TrimSpace(fmt.Sprint(env.Payload["correlation_id"])); correlationID != "" && correlationID != "<nil>" {
		req.CorrelationID = correlationID
	}
	req.CheckIDs = channelPayloadIntList(env.Payload["check_ids"])
	req.TimeoutMs = intFromAny(env.Payload["timeout_ms"])
	return req
}

func agentlessCommandScope(cmd usecase.ControlCommand) string {
	if len(cmd.CheckIDs) > 0 {
		return "agentless_check"
	}
	return "agentless"
}
