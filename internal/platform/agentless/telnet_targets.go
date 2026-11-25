package agentless

import (
	"bytes"
	"encoding/json"
	"io"
	"net/netip"

	"github.com/you/aiceberg_agent/internal/data/local/safefile"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type telnetTarget struct {
	ClientID   int    `json:"client_id"`
	AssetID    int    `json:"asset_id"`
	CheckID    int    `json:"check_id"`
	EndpointID int    `json:"endpoint_id"`
	IP         string `json:"ip"`
	Port       int    `json:"port"`
}

func telnetEndpoint(job entities.AgentlessJob) (netip.AddrPort, bool) {
	endpoint := job.Endpoint
	if endpoint == nil || endpoint.ID < 1 || endpoint.Tipo != "ip" {
		return netip.AddrPort{}, false
	}
	ip, err := netip.ParseAddr(endpoint.Endereco)
	if err != nil || ip.Zone() != "" || !probeHTTPAddressAllowed(endpoint.Endereco, ip.Unmap()) {
		return netip.AddrPort{}, false
	}
	port := 23
	if endpoint.Porta != nil {
		port = *endpoint.Porta
	}
	if port < 1 || port > 65535 {
		return netip.AddrPort{}, false
	}
	return netip.AddrPortFrom(ip.Unmap(), uint16(port)), true
}

func telnetTargetAllowed(path string, job entities.AgentlessJob, address netip.AddrPort) bool {
	if job.ClienteID < 1 || job.AtivoID < 1 || job.CheckID < 1 || job.ProbePolicy == nil {
		return false
	}
	data, err := safefile.ReadPrivate(path, 128<<10)
	if err != nil {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var file struct {
		Targets []telnetTarget `json:"targets"`
	}
	if decoder.Decode(&file) != nil || decoder.Decode(new(any)) != io.EOF || len(file.Targets) > 256 {
		return false
	}
	expected := telnetTarget{ClientID: job.ClienteID, AssetID: job.AtivoID, CheckID: job.CheckID,
		EndpointID: job.Endpoint.ID, IP: address.Addr().String(), Port: int(address.Port())}
	matches := 0
	for _, candidate := range file.Targets {
		if candidate == expected {
			matches++
		}
	}
	return matches == 1
}
