package agentless

import (
	"net/url"
	"strconv"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

// ActiveProbeTarget exposes the effective destination of HTTP, TCP and TLS
// probes to the HUB admission policy, including legacy check configuration.
func ActiveProbeTarget(job entities.AgentlessJob) (host string, port int, handled bool) {
	switch probeProtocol(job.Tipo) {
	case "tcp":
		return endpointHost(job), endpointPort(job, 0), true
	case "tls":
		return endpointHost(job), endpointPort(job, 443), true
	case "http":
		host, port := httpProbeTarget(job)
		return host, port, true
	default:
		return "", 0, false
	}
}

func httpProbeTarget(job entities.AgentlessJob) (string, int) {
	raw, err := buildHTTPURL(job, false)
	if err != nil {
		return "", 0
	}
	target, err := url.Parse(raw)
	if err != nil || target.Hostname() == "" {
		return "", 0
	}
	if explicit := target.Port(); explicit != "" {
		port, _ := strconv.Atoi(explicit)
		return target.Hostname(), port
	}
	switch target.Scheme {
	case "http":
		return target.Hostname(), 80
	case "https":
		return target.Hostname(), 443
	default:
		return target.Hostname(), 0
	}
}
