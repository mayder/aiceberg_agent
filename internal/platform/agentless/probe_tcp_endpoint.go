package agentless

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"regexp"
	"strings"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

var probeTCPHostname = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)

func probeTCPEndpoint(endpoint *entities.AgentlessEndpoint, defaultPort int) (string, int, bool) {
	if endpoint == nil || endpoint.ID < 1 {
		return "", 0, false
	}
	host := endpoint.Endereco
	ip, err := netip.ParseAddr(host)
	switch endpoint.Tipo {
	case "ip":
		if err != nil || ip.Zone() != "" || !probeHTTPAddressAllowed(host, ip.Unmap()) {
			return "", 0, false
		}
		host = ip.String()
	case "fqdn":
		if len(host) > 253 || !probeTCPHostname.MatchString(host) {
			return "", 0, false
		}
		host = strings.ToLower(host)
	default:
		return "", 0, false
	}
	port := defaultPort
	if endpoint.Porta != nil {
		port = *endpoint.Porta
	}
	return host, port, port > 0 && port <= 65535
}

func probeTCPHostLookup(ctx context.Context, host string) ([]string, error) {
	ipList, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	addresses := make([]string, 0, len(ipList))
	for _, address := range ipList {
		ip := address.Unmap()
		if probeHTTPAddressAllowed(host, ip) {
			addresses = append(addresses, ip.String())
		}
	}
	if len(addresses) == 0 {
		return nil, errors.New("probe_tcp_endpoint_unreachable")
	}
	return addresses, nil
}
