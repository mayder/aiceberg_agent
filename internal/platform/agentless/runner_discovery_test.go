package agentless

import (
	"context"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestParseDiscoveryPolicyBasic(t *testing.T) {
	cfg := map[string]any{
		"allowed_cidrs":   []any{"172.31.0.0/30"},
		"blocked_cidrs":   []string{"172.31.0.2/32"},
		"rate_limit_pps":  99999,
		"max_hosts":       99999,
		"window_start":    "22:00",
		"window_end":      "06:00",
		"window_timezone": "UTC",
		"allow_arp":       true,
		"allow_snmp":      false,
	}

	p, err := parseDiscoveryPolicy(cfg, 1500)
	if err != nil {
		t.Fatalf("erro inesperado ao parsear policy: %v", err)
	}
	if len(p.AllowedCIDRs) != 1 || p.AllowedCIDRs[0] != "172.31.0.0/30" {
		t.Fatalf("allowed cidrs inválido: %#v", p.AllowedCIDRs)
	}
	if len(p.BlockedCIDRs) != 1 || p.BlockedCIDRs[0] != "172.31.0.2/32" {
		t.Fatalf("blocked cidrs inválido: %#v", p.BlockedCIDRs)
	}
	if p.RateLimitPPS != 2000 {
		t.Fatalf("rate_limit_pps deveria ser clampado em 2000, obteve %d", p.RateLimitPPS)
	}
	if p.MaxHosts != 4096 {
		t.Fatalf("max_hosts deveria ser clampado em 4096, obteve %d", p.MaxHosts)
	}
	if p.AllowSNMP {
		t.Fatalf("allow_snmp deveria ser false")
	}
}

func TestDiscoveryWindowAllowedOvernight(t *testing.T) {
	p := discoveryPolicy{
		WindowStart:    "22:00",
		WindowEnd:      "06:00",
		WindowTimezone: "UTC",
	}

	ok, _ := discoveryWindowAllowed(p, time.Date(2026, 2, 22, 23, 0, 0, 0, time.UTC))
	if !ok {
		t.Fatalf("janela overnight deveria permitir 23:00")
	}
	ok, _ = discoveryWindowAllowed(p, time.Date(2026, 2, 22, 7, 0, 0, 0, time.UTC))
	if ok {
		t.Fatalf("janela overnight não deveria permitir 07:00")
	}
}

func TestBuildDiscoveryHostsWithBlockAndLimit(t *testing.T) {
	hosts, truncated := buildDiscoveryHosts(
		[]string{"172.31.0.0/29"},
		[]string{"172.31.0.3/32"},
		3,
	)
	if len(hosts) != 3 {
		t.Fatalf("esperava 3 hosts (limite), obteve %d: %#v", len(hosts), hosts)
	}
	for _, h := range hosts {
		if h == "172.31.0.3" {
			t.Fatalf("host bloqueado não deveria aparecer: %#v", hosts)
		}
	}
	if truncated <= 0 {
		t.Fatalf("esperava truncamento > 0, obteve %d", truncated)
	}
}

func TestRunJobDiscoveryPolicyInvalid(t *testing.T) {
	job := entities.AgentlessJob{
		CheckID:   1001,
		Tipo:      "discovery_assisted",
		TimeoutMs: 1200,
		Config: entities.AgentlessConfig{
			"allowed_cidrs": []string{"cidr-invalido"},
		},
	}

	obs := RunJob(context.Background(), job)
	if obs.Status != "fail" {
		t.Fatalf("status esperado fail, obteve %s", obs.Status)
	}
	if obs.Code != "discovery_policy_invalid" {
		t.Fatalf("code esperado discovery_policy_invalid, obteve %s", obs.Code)
	}
}
