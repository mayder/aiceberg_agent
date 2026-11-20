package entities

type RelaySNMPJob struct {
	Status      string        `json:"status"`
	ExecutionID int           `json:"execution_id"`
	ExecutorID  int           `json:"executor_id"`
	OwnerHubID  int           `json:"owner_hub_id"`
	ExpiresAt   int64         `json:"expires_at"`
	Job         *AgentlessJob `json:"job"`
}
