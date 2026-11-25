package agentless

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

var probeHTTPTransport = &http.Transport{
	DialContext: probeHTTPDial, MaxResponseHeaderBytes: 32 << 10, IdleConnTimeout: 30 * time.Second,
	TLSHandshakeTimeout: 5 * time.Second, MaxIdleConns: 16, MaxIdleConnsPerHost: 2,
}

var probeHTTPClient = &http.Client{Transport: probeHTTPTransport, CheckRedirect: func(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}}

func probeHTTPURL(endpoint *entities.AgentlessEndpoint) (string, error) {
	if endpoint == nil || endpoint.Tipo != "url" {
		return "", monitoring.ErrProbePolicy
	}
	target, err := url.Parse(endpoint.Endereco)
	if err != nil || target.Hostname() == "" || target.User != nil || target.RawQuery != "" ||
		target.ForceQuery || target.Fragment != "" || strings.Contains(endpoint.Endereco, "#") ||
		(target.Scheme != "http" && target.Scheme != "https") {
		return "", monitoring.ErrProbePolicy
	}
	return target.String(), nil
}

func probeHTTPDial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	dialer := net.Dialer{}
	for _, raw := range addresses {
		ip := raw.Unmap()
		if !probeHTTPAddressAllowed(host, ip) {
			continue
		}
		connection, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return connection, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("probe_http_endpoint_unreachable")
}

func probeHTTPAddressAllowed(host string, ip netip.Addr) bool {
	if ip.IsLoopback() {
		literal, err := netip.ParseAddr(host)
		return strings.EqualFold(host, "localhost") || (err == nil && literal.Unmap() == ip)
	}
	return ip.IsGlobalUnicast() && ip != netip.MustParseAddr("100.100.100.200") && ip != netip.MustParseAddr("fd00:ec2::254")
}
