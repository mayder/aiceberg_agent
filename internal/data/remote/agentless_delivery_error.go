package remote

import "fmt"

// No response body: errors must never expose observation payloads or credentials.
type AgentlessDeliveryError struct {
	HTTPStatus      int
	PayloadRejected bool
}

func (e *AgentlessDeliveryError) Error() string {
	return fmt.Sprintf("agentless delivery not acknowledged (http=%d payload_rejected=%t)", e.HTTPStatus, e.PayloadRejected)
}
