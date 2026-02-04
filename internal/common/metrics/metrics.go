package metrics

import "sync/atomic"

var invalidEnvelopes atomic.Int64
var agentlessJobs atomic.Int64

// IncInvalidEnvelope incrementa o total de envelopes inválidos descartados.
func IncInvalidEnvelope() {
	invalidEnvelopes.Add(1)
}

// AddAgentlessJobs soma a quantidade de jobs agentless executados.
func AddAgentlessJobs(n int) {
	if n <= 0 {
		return
	}
	agentlessJobs.Add(int64(n))
}

// InvalidEnvelopesTotal retorna o total de envelopes inválidos descartados.
func InvalidEnvelopesTotal() int64 {
	return invalidEnvelopes.Load()
}

// AgentlessJobsTotal retorna o total de jobs agentless executados.
func AgentlessJobsTotal() int64 {
	return agentlessJobs.Load()
}
