package agentless

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	gosnmp "github.com/gosnmp/gosnmp"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type Result struct {
	Status    string
	LatencyMs int
	Code      string
	Message   string
	Payload   map[string]any
}

type discoveryPolicy struct {
	AllowedCIDRs   []string
	BlockedCIDRs   []string
	RateLimitPPS   int
	MaxHosts       int
	AllowARP       bool
	AllowSNMP      bool
	AllowLLDP      bool
	AllowCDP       bool
	WindowStart    string
	WindowEnd      string
	WindowTimezone string
	WindowWeekdays map[time.Weekday]struct{}
	SNMPCommunity  string
	SNMPVersion    string
	SNMPPort       int
	SNMPTimeoutMs  int
	SNMPRetries    int
}

func RunJob(ctx context.Context, job entities.AgentlessJob) entities.AgentlessObservation {
	started := time.Now()
	res := Result{Status: "fail"}

	switch strings.ToLower(job.Tipo) {
	case "icmp":
		res = runICMP(ctx, job)
	case "tcp":
		res = runTCP(ctx, job)
	case "http":
		res = runHTTP(ctx, job, false)
	case "dns":
		res = runDNS(ctx, job)
	case "tls":
		res = runTLS(ctx, job)
	case "snmp":
		res = runSNMP(ctx, job)
	case "discovery_assisted", "discovery":
		res = runDiscoveryAssisted(ctx, job)
	default:
		res = Result{Status: "fail", Code: "unknown_type", Message: "tipo nao suportado"}
	}

	obs := entities.AgentlessObservation{
		ID:         newID("obs"),
		CheckID:    job.CheckID,
		Status:     res.Status,
		LatencyMs:  res.LatencyMs,
		Code:       trimString(res.Code, 32),
		Message:    trimString(res.Message, 255),
		Payload:    res.Payload,
		ObservedAt: time.Now(),
		CreatedAt:  time.Now(),
	}

	if job.Endpoint != nil {
		id := job.Endpoint.ID
		obs.EndpointID = &id
	}

	if obs.LatencyMs == 0 {
		obs.LatencyMs = int(time.Since(started).Milliseconds())
	}

	return obs
}

func runICMP(ctx context.Context, job entities.AgentlessJob) Result {
	target := endpointHost(job)
	if target == "" {
		return Result{Status: "fail", Code: "missing_endpoint", Message: "endpoint ausente"}
	}
	timeout := time.Millisecond * time.Duration(maxInt(job.TimeoutMs, 1000))
	latency, out, err := pingOnce(ctx, target, timeout)
	if err != nil {
		return Result{Status: "fail", Code: "ping_fail", Message: err.Error(), Payload: map[string]any{"output": out}}
	}
	return Result{Status: "ok", LatencyMs: latency, Payload: map[string]any{"output": out}}
}

func runTCP(ctx context.Context, job entities.AgentlessJob) Result {
	host := endpointHost(job)
	port := endpointPort(job, 0)
	if host == "" || port == 0 {
		return Result{Status: "fail", Code: "missing_endpoint", Message: "endpoint/porta ausente"}
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()
	d := net.Dialer{Timeout: time.Millisecond * time.Duration(maxInt(job.TimeoutMs, 1000))}
	conn, err := d.DialContext(ctx, "tcp", addr)
	latency := int(time.Since(start).Milliseconds())
	if conn != nil {
		_ = conn.Close()
	}
	if err != nil {
		return Result{Status: "fail", LatencyMs: latency, Code: "connect_fail", Message: err.Error()}
	}
	return Result{Status: "ok", LatencyMs: latency}
}

func runHTTP(ctx context.Context, job entities.AgentlessJob, tlsOnly bool) Result {
	endpointURL, err := buildHTTPURL(job, tlsOnly)
	if err != nil {
		return Result{Status: "fail", Code: "invalid_url", Message: err.Error()}
	}
	method := getString(map[string]any(job.Config), "method", "GET")
	client := http.Client{Timeout: time.Millisecond * time.Duration(maxInt(job.TimeoutMs, 5000))}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, method, endpointURL, nil)
	if err != nil {
		return Result{Status: "fail", Code: "http_request", Message: err.Error()}
	}
	resp, err := client.Do(req)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return Result{Status: "fail", LatencyMs: latency, Code: "http_fail", Message: err.Error()}
	}
	defer resp.Body.Close()
	payload := map[string]any{
		"status_code": resp.StatusCode,
		"status":      resp.Status,
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return Result{Status: "ok", LatencyMs: latency, Payload: payload}
	}
	return Result{Status: "fail", LatencyMs: latency, Code: "http_status", Message: resp.Status, Payload: payload}
}

func runDNS(ctx context.Context, job entities.AgentlessJob) Result {
	host := endpointHost(job)
	if host == "" {
		return Result{Status: "fail", Code: "missing_endpoint", Message: "endpoint ausente"}
	}
	timeout := time.Millisecond * time.Duration(maxInt(job.TimeoutMs, 2000))
	resolver := net.Resolver{}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	addrs, err := resolver.LookupHost(ctx, host)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return Result{Status: "fail", LatencyMs: latency, Code: "dns_fail", Message: err.Error()}
	}
	return Result{Status: "ok", LatencyMs: latency, Payload: map[string]any{"addrs": addrs}}
}

func runTLS(ctx context.Context, job entities.AgentlessJob) Result {
	host := endpointHost(job)
	port := endpointPort(job, 443)
	if host == "" {
		return Result{Status: "fail", Code: "missing_endpoint", Message: "endpoint ausente"}
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()
	d := net.Dialer{Timeout: time.Millisecond * time.Duration(maxInt(job.TimeoutMs, 5000))}
	conf := &tls.Config{InsecureSkipVerify: true}
	if job.Endpoint != nil && job.Endpoint.TLSSNI != "" {
		conf.ServerName = job.Endpoint.TLSSNI
	}
	conn, err := tls.DialWithDialer(&d, "tcp", addr, conf)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return Result{Status: "fail", LatencyMs: latency, Code: "tls_fail", Message: err.Error()}
	}
	state := conn.ConnectionState()
	_ = conn.Close()

	payload := map[string]any{}
	if len(state.PeerCertificates) > 0 {
		cert := state.PeerCertificates[0]
		days := int(time.Until(cert.NotAfter).Hours() / 24)
		payload["subject"] = cert.Subject.String()
		payload["issuer"] = cert.Issuer.String()
		payload["not_before"] = cert.NotBefore.Format(time.RFC3339)
		payload["not_after"] = cert.NotAfter.Format(time.RFC3339)
		payload["days_to_expire"] = days
		if days < 0 {
			return Result{Status: "fail", LatencyMs: latency, Code: "cert_expired", Message: "certificado expirado", Payload: payload}
		}
	}
	return Result{Status: "ok", LatencyMs: latency, Payload: payload}
}

func runSNMP(ctx context.Context, job entities.AgentlessJob) Result {
	return runSNMPCollection(ctx, job)
}

func runDiscoveryAssisted(ctx context.Context, job entities.AgentlessJob) Result {
	cfg := map[string]any(job.Config)
	policy, err := parseDiscoveryPolicy(cfg, job.TimeoutMs)
	if err != nil {
		return Result{Status: "fail", Code: "discovery_policy_invalid", Message: err.Error()}
	}

	allowed, reason := discoveryWindowAllowed(policy, time.Now())
	if !allowed {
		return Result{
			Status:  "fail",
			Code:    "discovery_window_closed",
			Message: reason,
			Payload: map[string]any{
				"scope": map[string]any{
					"allowed_cidrs": policy.AllowedCIDRs,
					"blocked_cidrs": policy.BlockedCIDRs,
					"window_start":  policy.WindowStart,
					"window_end":    policy.WindowEnd,
					"timezone":      policy.WindowTimezone,
				},
			},
		}
	}

	hosts, truncated := buildDiscoveryHosts(policy.AllowedCIDRs, policy.BlockedCIDRs, policy.MaxHosts)
	if len(hosts) == 0 {
		return Result{
			Status:  "fail",
			Code:    "discovery_scope_empty",
			Message: "nenhum host elegivel para descoberta",
			Payload: map[string]any{
				"scope": map[string]any{
					"allowed_cidrs": policy.AllowedCIDRs,
					"blocked_cidrs": policy.BlockedCIDRs,
					"max_hosts":     policy.MaxHosts,
				},
			},
		}
	}

	var (
		scanned       int
		discovered    int
		arpSeen       int
		snmpOk        int
		snmpFail      int
		unsupported   int
		hostRows      []map[string]any
		discoveredIPs []string
	)

	const hostRowsLimit = 256
	timeout := time.Millisecond * time.Duration(maxInt(job.TimeoutMs, 500))
	if timeout < 500*time.Millisecond {
		timeout = 500 * time.Millisecond
	}
	throttle := time.Duration(0)
	if policy.RateLimitPPS > 0 {
		throttle = time.Second / time.Duration(policy.RateLimitPPS)
	}
	nextAllowed := time.Now()

	for _, host := range hosts {
		select {
		case <-ctx.Done():
			return Result{
				Status:  "fail",
				Code:    "discovery_canceled",
				Message: ctx.Err().Error(),
				Payload: map[string]any{
					"stats": map[string]any{
						"scanned_hosts":    scanned,
						"discovered_hosts": discovered,
					},
				},
			}
		default:
		}

		if throttle > 0 {
			now := time.Now()
			if now.Before(nextAllowed) {
				wait := nextAllowed.Sub(now)
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					timer.Stop()
					return Result{Status: "fail", Code: "discovery_canceled", Message: ctx.Err().Error()}
				case <-timer.C:
				}
			}
			nextAllowed = time.Now().Add(throttle)
		}

		scanned++
		row := map[string]any{"ip": host}
		hostFound := false

		if policy.AllowARP {
			arp := probeARP(ctx, host, timeout)
			row["arp"] = arp
			if seen, _ := arp["seen"].(bool); seen {
				hostFound = true
				arpSeen++
			}
			if status, _ := arp["status"].(string); status == "unsupported" {
				unsupported++
			}
		}

		if policy.AllowSNMP {
			snmpProbe := probeDiscoverySNMP(ctx, host, policy)
			row["snmp"] = snmpProbe
			status, _ := snmpProbe["status"].(string)
			switch status {
			case "ok":
				hostFound = true
				snmpOk++
			case "unsupported":
				unsupported++
			case "fail":
				snmpFail++
			}
		}

		if policy.AllowLLDP {
			row["lldp"] = map[string]any{
				"status":  "unsupported",
				"message": "coleta LLDP ainda não implementada no agente",
			}
			unsupported++
		}
		if policy.AllowCDP {
			row["cdp"] = map[string]any{
				"status":  "unsupported",
				"message": "coleta CDP ainda não implementada no agente",
			}
			unsupported++
		}

		if hostFound {
			discovered++
			discoveredIPs = append(discoveredIPs, host)
		}
		if len(hostRows) < hostRowsLimit {
			hostRows = append(hostRows, row)
		}
	}

	payload := map[string]any{
		"scope": map[string]any{
			"allowed_cidrs":   policy.AllowedCIDRs,
			"blocked_cidrs":   policy.BlockedCIDRs,
			"rate_limit_pps":  policy.RateLimitPPS,
			"max_hosts":       policy.MaxHosts,
			"allow_arp":       policy.AllowARP,
			"allow_snmp":      policy.AllowSNMP,
			"allow_lldp":      policy.AllowLLDP,
			"allow_cdp":       policy.AllowCDP,
			"window_start":    policy.WindowStart,
			"window_end":      policy.WindowEnd,
			"window_timezone": policy.WindowTimezone,
		},
		"stats": map[string]any{
			"scanned_hosts":      scanned,
			"discovered_hosts":   discovered,
			"arp_seen_hosts":     arpSeen,
			"snmp_ok_hosts":      snmpOk,
			"snmp_fail_hosts":    snmpFail,
			"unsupported_probes": unsupported,
			"truncated_hosts":    truncated,
			"sampled_hosts":      len(hostRows),
		},
		"discovered_ips": discoveredIPs,
		"hosts":          hostRows,
	}

	if discovered == 0 {
		return Result{
			Status:  "fail",
			Code:    "discovery_no_match",
			Message: fmt.Sprintf("descoberta concluída sem hosts detectados (escaneados=%d)", scanned),
			Payload: payload,
		}
	}

	return Result{
		Status:  "ok",
		Code:    "discovery_ok",
		Message: fmt.Sprintf("descoberta concluída: %d host(s) detectado(s) em %d analisado(s)", discovered, scanned),
		Payload: payload,
	}
}

func parseDiscoveryPolicy(cfg map[string]any, timeoutMs int) (discoveryPolicy, error) {
	p := discoveryPolicy{
		RateLimitPPS:   20,
		MaxHosts:       512,
		AllowARP:       true,
		AllowSNMP:      true,
		AllowLLDP:      false,
		AllowCDP:       false,
		WindowTimezone: "UTC",
		SNMPVersion:    "2c",
		SNMPPort:       161,
		SNMPTimeoutMs:  maxInt(timeoutMs, 1200),
		SNMPRetries:    0,
		WindowWeekdays: map[time.Weekday]struct{}{},
	}
	if p.SNMPTimeoutMs <= 0 {
		p.SNMPTimeoutMs = 1200
	}

	source := cfg
	if nested, ok := asMap(cfg["discovery_policy"]); ok {
		source = mergeMap(copyMap(cfg), nested)
	} else if nested, ok := asMap(cfg["policy"]); ok {
		source = mergeMap(copyMap(cfg), nested)
	}

	p.AllowedCIDRs = normalizeCIDRList(stringSliceFromAny(firstNonNil(source["allowed_cidrs"], source["cidrs"])))
	p.BlockedCIDRs = normalizeCIDRList(stringSliceFromAny(source["blocked_cidrs"]))

	if len(p.AllowedCIDRs) == 0 {
		return p, errors.New("allowed_cidrs é obrigatório para descoberta assistida")
	}
	for _, cidr := range p.AllowedCIDRs {
		if !isValidCIDR(cidr) {
			return p, fmt.Errorf("CIDR permitido inválido: %s", cidr)
		}
	}
	for _, cidr := range p.BlockedCIDRs {
		if !isValidCIDR(cidr) {
			return p, fmt.Errorf("CIDR bloqueado inválido: %s", cidr)
		}
	}

	if v, ok := toInt(source["rate_limit_pps"]); ok {
		if v < 1 {
			v = 1
		}
		if v > 2000 {
			v = 2000
		}
		p.RateLimitPPS = v
	}
	if v, ok := toInt(source["max_hosts"]); ok {
		if v < 1 {
			v = 1
		}
		if v > 4096 {
			v = 4096
		}
		p.MaxHosts = v
	}
	if v, ok := getBool(source, "allow_arp"); ok {
		p.AllowARP = v
	}
	if v, ok := getBool(source, "allow_snmp"); ok {
		p.AllowSNMP = v
	}
	if v, ok := getBool(source, "allow_lldp"); ok {
		p.AllowLLDP = v
	}
	if v, ok := getBool(source, "allow_cdp"); ok {
		p.AllowCDP = v
	}
	p.WindowStart = strings.TrimSpace(getString(source, "window_start", ""))
	p.WindowEnd = strings.TrimSpace(getString(source, "window_end", ""))
	p.WindowTimezone = strings.TrimSpace(getString(source, "window_timezone", p.WindowTimezone))
	if p.WindowTimezone == "" {
		p.WindowTimezone = "UTC"
	}
	if _, err := time.LoadLocation(p.WindowTimezone); err != nil {
		return p, fmt.Errorf("window_timezone inválido: %s", p.WindowTimezone)
	}
	if (p.WindowStart != "" || p.WindowEnd != "") && (!isValidHHMM(p.WindowStart) || !isValidHHMM(p.WindowEnd)) {
		return p, errors.New("window_start/window_end devem estar no formato HH:MM")
	}

	for _, wd := range parseWeekdays(source["window_weekdays"]) {
		p.WindowWeekdays[wd] = struct{}{}
	}

	snmpCfg := source
	if nested, ok := asMap(source["snmp"]); ok {
		snmpCfg = mergeMap(copyMap(source), nested)
	}
	p.SNMPCommunity = strings.TrimSpace(getString(snmpCfg, "community", getString(source, "snmp_community", "")))
	p.SNMPVersion = strings.TrimSpace(strings.ToLower(getString(snmpCfg, "version", p.SNMPVersion)))
	if p.SNMPVersion == "" {
		p.SNMPVersion = "2c"
	}
	if v, ok := toInt(firstNonNil(snmpCfg["port"], source["snmp_port"])); ok && v > 0 {
		p.SNMPPort = v
	}
	if v, ok := toInt(firstNonNil(snmpCfg["timeout_ms"], source["snmp_timeout_ms"])); ok && v > 0 {
		p.SNMPTimeoutMs = v
	}
	if v, ok := toInt(firstNonNil(snmpCfg["retries"], source["snmp_retries"])); ok && v >= 0 {
		if v > 5 {
			v = 5
		}
		p.SNMPRetries = v
	}
	return p, nil
}

func discoveryWindowAllowed(policy discoveryPolicy, now time.Time) (bool, string) {
	if policy.WindowStart == "" || policy.WindowEnd == "" {
		if len(policy.WindowWeekdays) == 0 {
			return true, ""
		}
		loc, err := time.LoadLocation(policy.WindowTimezone)
		if err != nil {
			return false, "timezone inválido na política"
		}
		n := now.In(loc)
		if _, ok := policy.WindowWeekdays[n.Weekday()]; !ok {
			return false, "fora dos dias permitidos para descoberta"
		}
		return true, ""
	}

	loc, err := time.LoadLocation(policy.WindowTimezone)
	if err != nil {
		return false, "timezone inválido na política"
	}
	n := now.In(loc)
	if len(policy.WindowWeekdays) > 0 {
		if _, ok := policy.WindowWeekdays[n.Weekday()]; !ok {
			return false, "fora dos dias permitidos para descoberta"
		}
	}

	startMin, okStart := parseHHMM(policy.WindowStart)
	endMin, okEnd := parseHHMM(policy.WindowEnd)
	if !okStart || !okEnd {
		return false, "janela de descoberta inválida"
	}
	curMin := n.Hour()*60 + n.Minute()
	if startMin == endMin {
		return true, ""
	}
	if startMin < endMin {
		if curMin >= startMin && curMin < endMin {
			return true, ""
		}
		return false, "fora da janela permitida de descoberta"
	}
	if curMin >= startMin || curMin < endMin {
		return true, ""
	}
	return false, "fora da janela permitida de descoberta"
}

func buildDiscoveryHosts(allowedCIDRs, blockedCIDRs []string, maxHosts int) ([]string, int) {
	if maxHosts <= 0 {
		maxHosts = 512
	}
	blocked := make([]*net.IPNet, 0, len(blockedCIDRs))
	for _, raw := range blockedCIDRs {
		_, n, err := net.ParseCIDR(raw)
		if err == nil && n != nil {
			blocked = append(blocked, n)
		}
	}

	seen := make(map[string]struct{}, maxHosts)
	hosts := make([]string, 0, maxHosts)
	truncated := 0
	for _, raw := range allowedCIDRs {
		scanLimit := maxHosts * 4
		if scanLimit < 1024 {
			scanLimit = 1024
		}
		expanded, err := expandCIDRHosts(raw, scanLimit)
		if err != nil {
			continue
		}
		for _, host := range expanded {
			ip := net.ParseIP(host)
			if ip == nil {
				continue
			}
			if isIPInBlocked(ip, blocked) {
				continue
			}
			if _, ok := seen[host]; ok {
				continue
			}
			if len(hosts) >= maxHosts {
				truncated++
				continue
			}
			seen[host] = struct{}{}
			hosts = append(hosts, host)
		}
	}
	sort.Strings(hosts)
	return hosts, truncated
}

func expandCIDRHosts(raw string, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	ip, n, err := net.ParseCIDR(raw)
	if err != nil || n == nil {
		return nil, fmt.Errorf("cidr inválido: %s", raw)
	}
	base := ip.To4()
	if base == nil {
		return nil, fmt.Errorf("apenas IPv4 suportado em descoberta assistida: %s", raw)
	}

	ones, bits := n.Mask.Size()
	if bits != 32 {
		return nil, fmt.Errorf("máscara não suportada: %s", raw)
	}

	start := ipToUint32(base.Mask(n.Mask))
	var out []string
	switch {
	case ones == 32:
		out = append(out, uint32ToIP(start).String())
	case ones == 31:
		out = append(out, uint32ToIP(start).String(), uint32ToIP(start+1).String())
	default:
		hostBits := uint32(32 - ones)
		broadcast := start + (1 << hostBits) - 1
		for cur := start + 1; cur < broadcast; cur++ {
			out = append(out, uint32ToIP(cur).String())
			if len(out) >= limit {
				break
			}
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func probeARP(ctx context.Context, host string, timeout time.Duration) map[string]any {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch runtime.GOOS {
	case "linux":
		out, err := exec.CommandContext(timeoutCtx, "ip", "neigh", "show", "to", host).CombinedOutput()
		text := strings.TrimSpace(string(out))
		if err != nil && text == "" {
			return map[string]any{"status": "fail", "seen": false, "error": trimString(err.Error(), 180)}
		}
		lower := strings.ToLower(text)
		seen := text != "" && !strings.Contains(lower, "incomplete") && !strings.Contains(lower, "failed")
		mac := parseLinuxNeighMAC(text)
		return map[string]any{"status": "ok", "seen": seen, "mac": mac}
	case "darwin":
		out, err := exec.CommandContext(timeoutCtx, "arp", "-n", host).CombinedOutput()
		text := strings.TrimSpace(string(out))
		if err != nil && text == "" {
			return map[string]any{"status": "fail", "seen": false, "error": trimString(err.Error(), 180)}
		}
		lower := strings.ToLower(text)
		seen := text != "" && !strings.Contains(lower, "(incomplete)")
		mac := parseDarwinArpMAC(text)
		return map[string]any{"status": "ok", "seen": seen, "mac": mac}
	default:
		return map[string]any{"status": "unsupported", "seen": false, "message": "ARP probe indisponível neste SO"}
	}
}

func probeDiscoverySNMP(ctx context.Context, host string, policy discoveryPolicy) map[string]any {
	if !policy.AllowSNMP {
		return map[string]any{"status": "skipped", "message": "SNMP desativado pela política"}
	}
	if strings.TrimSpace(policy.SNMPCommunity) == "" {
		return map[string]any{"status": "skipped", "message": "community SNMP ausente"}
	}
	if strings.TrimSpace(policy.SNMPVersion) != "2c" {
		return map[string]any{"status": "unsupported", "message": "somente SNMP v2c suportado neste fluxo"}
	}
	if err := ctx.Err(); err != nil {
		return map[string]any{"status": "fail", "error": err.Error()}
	}

	sn := &gosnmp.GoSNMP{
		Target:    host,
		Port:      uint16(policy.SNMPPort),
		Version:   gosnmp.Version2c,
		Community: policy.SNMPCommunity,
		Timeout:   time.Millisecond * time.Duration(maxInt(policy.SNMPTimeoutMs, 600)),
		Retries:   maxInt(policy.SNMPRetries, 0),
		MaxOids:   gosnmp.MaxOids,
	}
	start := time.Now()
	if err := sn.Connect(); err != nil {
		return map[string]any{
			"status":     "fail",
			"latency_ms": int(time.Since(start).Milliseconds()),
			"error":      trimString(err.Error(), 180),
		}
	}
	defer func() {
		if sn.Conn != nil {
			_ = sn.Conn.Close()
		}
	}()

	resp, err := sn.Get([]string{"1.3.6.1.2.1.1.5.0", "1.3.6.1.2.1.1.1.0"})
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return map[string]any{
			"status":     "fail",
			"latency_ms": latency,
			"error":      trimString(err.Error(), 180),
		}
	}

	out := map[string]any{
		"status":     "ok",
		"latency_ms": latency,
	}
	for _, v := range snmpVariables(resp) {
		val := snmpValueToAny(v)
		switch v.Name {
		case ".1.3.6.1.2.1.1.5.0", "1.3.6.1.2.1.1.5.0":
			out["sys_name"] = val
		case ".1.3.6.1.2.1.1.1.0", "1.3.6.1.2.1.1.1.0":
			out["sys_descr"] = val
		}
	}
	return out
}

func isIPInBlocked(ip net.IP, blocked []*net.IPNet) bool {
	for _, n := range blocked {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func normalizeCIDRList(list []string) []string {
	out := make([]string, 0, len(list))
	seen := map[string]struct{}{}
	for _, item := range list {
		cidr := strings.TrimSpace(item)
		if cidr == "" {
			continue
		}
		if _, ok := seen[cidr]; ok {
			continue
		}
		seen[cidr] = struct{}{}
		out = append(out, cidr)
	}
	return out
}

func stringSliceFromAny(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s := strings.TrimSpace(fmt.Sprintf("%v", item))
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		parts := strings.Split(t, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			s := strings.TrimSpace(part)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func parseWeekdays(v any) []time.Weekday {
	items := stringSliceFromAny(v)
	out := make([]time.Weekday, 0, len(items))
	for _, item := range items {
		switch strings.ToLower(strings.TrimSpace(item)) {
		case "0", "sun", "domingo":
			out = append(out, time.Sunday)
		case "1", "mon", "segunda":
			out = append(out, time.Monday)
		case "2", "tue", "terça", "terca":
			out = append(out, time.Tuesday)
		case "3", "wed", "quarta":
			out = append(out, time.Wednesday)
		case "4", "thu", "quinta":
			out = append(out, time.Thursday)
		case "5", "fri", "sexta":
			out = append(out, time.Friday)
		case "6", "sat", "sábado", "sabado":
			out = append(out, time.Saturday)
		}
	}
	return out
}

func parseHHMM(v string) (int, bool) {
	v = strings.TrimSpace(v)
	if !isValidHHMM(v) {
		return 0, false
	}
	parts := strings.Split(v, ":")
	h, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	return h*60 + m, true
}

func isValidHHMM(v string) bool {
	if len(v) != 5 || v[2] != ':' {
		return false
	}
	h, errH := strconv.Atoi(v[:2])
	m, errM := strconv.Atoi(v[3:])
	if errH != nil || errM != nil {
		return false
	}
	return h >= 0 && h <= 23 && m >= 0 && m <= 59
}

func isValidCIDR(cidr string) bool {
	ip, n, err := net.ParseCIDR(strings.TrimSpace(cidr))
	return err == nil && ip != nil && n != nil
}

func ipToUint32(ip net.IP) uint32 {
	return binary.BigEndian.Uint32(ip.To4())
}

func uint32ToIP(v uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, v)
	return ip
}

func parseLinuxNeighMAC(text string) string {
	fields := strings.Fields(text)
	for i := 0; i < len(fields)-1; i++ {
		if strings.ToLower(fields[i]) == "lladdr" {
			return fields[i+1]
		}
	}
	return ""
}

func parseDarwinArpMAC(text string) string {
	start := strings.Index(text, " at ")
	if start < 0 {
		return ""
	}
	sub := text[start+4:]
	end := strings.Index(sub, " on ")
	if end < 0 {
		return strings.TrimSpace(sub)
	}
	return strings.TrimSpace(sub[:end])
}

func asMap(v any) (map[string]any, bool) {
	if v == nil {
		return nil, false
	}
	m, ok := v.(map[string]any)
	return m, ok
}

func copyMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mergeMap(base, extra map[string]any) map[string]any {
	out := copyMap(base)
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func firstNonNil(values ...any) any {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

func endpointHost(job entities.AgentlessJob) string {
	if job.Endpoint == nil {
		return ""
	}
	switch strings.ToLower(job.Endpoint.Tipo) {
	case "url":
		u, err := url.Parse(job.Endpoint.Endereco)
		if err == nil && u.Hostname() != "" {
			return u.Hostname()
		}
		return job.Endpoint.Endereco
	default:
		return job.Endpoint.Endereco
	}
}

func endpointPort(job entities.AgentlessJob, def int) int {
	if job.Endpoint == nil {
		return def
	}
	if job.Endpoint.Porta != nil && *job.Endpoint.Porta > 0 {
		return *job.Endpoint.Porta
	}
	if job.Config != nil {
		if v, ok := job.Config["port"]; ok {
			if n, ok := toInt(v); ok {
				return n
			}
		}
	}
	return def
}

func buildHTTPURL(job entities.AgentlessJob, forceTLS bool) (string, error) {
	if job.Endpoint == nil {
		return "", errors.New("endpoint ausente")
	}
	if strings.ToLower(job.Endpoint.Tipo) == "url" {
		u := job.Endpoint.Endereco
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			u = "http://" + u
		}
		return u, nil
	}
	scheme := getString(map[string]any(job.Config), "scheme", "http")
	if forceTLS {
		scheme = "https"
	}
	if v, ok := getBool(map[string]any(job.Config), "https"); ok && v {
		scheme = "https"
	}
	host := job.Endpoint.Endereco
	port := endpointPort(job, 0)
	path := getString(map[string]any(job.Config), "path", "/")
	if path == "" {
		path = "/"
	}
	if port > 0 {
		host = net.JoinHostPort(host, strconv.Itoa(port))
	}
	return (&url.URL{Scheme: scheme, Host: host, Path: path}).String(), nil
}

func pingOnce(ctx context.Context, target string, timeout time.Duration) (int, string, error) {
	var args []string
	ms := int(timeout.Milliseconds())
	if ms <= 0 {
		ms = 1000
	}
	switch runtime.GOOS {
	case "windows":
		args = []string{"-n", "1", "-w", strconv.Itoa(ms), target}
	case "darwin":
		args = []string{"-c", "1", "-W", strconv.Itoa(ms), target}
	default:
		sec := int(timeout.Seconds())
		if sec <= 0 {
			sec = 1
		}
		args = []string{"-c", "1", "-W", strconv.Itoa(sec), target}
	}
	cmd := exec.CommandContext(ctx, "ping", args...)
	out, err := cmd.CombinedOutput()
	latency := parsePingLatency(string(out))
	if err != nil {
		return latency, string(out), err
	}
	return latency, string(out), nil
}

func parsePingLatency(out string) int {
	out = strings.ToLower(out)
	for _, key := range []string{"time=", "tempo="} {
		if idx := strings.Index(out, key); idx >= 0 {
			sub := out[idx+len(key):]
			end := strings.IndexAny(sub, " ms,\n")
			if end > 0 {
				val := strings.TrimSpace(sub[:end])
				val = strings.Trim(val, "<>")
				if n, err := strconv.ParseFloat(val, 64); err == nil {
					return int(n)
				}
			}
		}
	}
	return 0
}

func newID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "_" + strconv.FormatInt(time.Now().UnixNano(), 10) + "_" + hex.EncodeToString(b)
}

func getString(m map[string]any, key, def string) string {
	if m == nil {
		return def
	}
	if v, ok := m[key]; ok {
		s, ok := v.(string)
		if ok && s != "" {
			return s
		}
	}
	return def
}

func getBool(m map[string]any, key string) (bool, bool) {
	if m == nil {
		return false, false
	}
	if v, ok := m[key]; ok {
		switch t := v.(type) {
		case bool:
			return t, true
		case string:
			return strings.ToLower(t) == "true", true
		case float64:
			return t != 0, true
		}
	}
	return false, false
}

func toInt(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int64:
		return int(t), true
	case float64:
		return int(t), true
	case string:
		if n, err := strconv.Atoi(t); err == nil {
			return n, true
		}
	}
	return 0, false
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func trimString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func mapSNMPAuth(name string) gosnmp.SnmpV3AuthProtocol {
	switch strings.ToLower(name) {
	case "md5":
		return gosnmp.MD5
	case "sha", "sha1":
		return gosnmp.SHA
	case "sha224":
		return gosnmp.SHA224
	case "sha256":
		return gosnmp.SHA256
	case "sha384":
		return gosnmp.SHA384
	case "sha512":
		return gosnmp.SHA512
	default:
		return gosnmp.NoAuth
	}
}

func mapSNMPPriv(name string) gosnmp.SnmpV3PrivProtocol {
	switch strings.ToLower(name) {
	case "des":
		return gosnmp.DES
	case "aes", "aes128":
		return gosnmp.AES
	case "aes192":
		return gosnmp.AES192
	case "aes256":
		return gosnmp.AES256
	case "aes256c":
		return gosnmp.AES256C
	default:
		return gosnmp.NoPriv
	}
}
