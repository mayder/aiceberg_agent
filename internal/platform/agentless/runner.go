package agentless

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
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
	method := getString(job.Config, "method", "GET")
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
	if job.SNMP == nil {
		return Result{Status: "fail", Code: "snmp_no_profile", Message: "perfil SNMP ausente"}
	}
	host := endpointHost(job)
	if host == "" {
		return Result{Status: "fail", Code: "missing_endpoint", Message: "endpoint ausente"}
	}
	port := job.SNMP.Port
	if port == 0 {
		port = 161
	}
	timeout := time.Millisecond * time.Duration(maxInt(job.SNMP.TimeoutMs, job.TimeoutMs))
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	sn := &gosnmp.GoSNMP{
		Target:         host,
		Port:           uint16(port),
		Timeout:        timeout,
		Retries:        maxInt(job.SNMP.Retries, job.Retries),
		MaxRepetitions: 1,
	}

	if strings.ToLower(job.SNMP.Version) == "v3" {
		sn.Version = gosnmp.Version3
		authProto := mapSNMPAuth(job.SNMP.V3AuthProtocol)
		privProto := mapSNMPPriv(job.SNMP.V3PrivProtocol)
		secParams := &gosnmp.UsmSecurityParameters{
			UserName:                 job.SNMP.V3User,
			AuthenticationProtocol:   authProto,
			AuthenticationPassphrase: job.SNMP.V3AuthPassword,
			PrivacyProtocol:          privProto,
			PrivacyPassphrase:        job.SNMP.V3PrivPassword,
		}
		sn.SecurityParameters = secParams
		sn.SecurityModel = gosnmp.UserSecurityModel
		if authProto == gosnmp.NoAuth {
			sn.MsgFlags = gosnmp.NoAuthNoPriv
		} else if privProto == gosnmp.NoPriv {
			sn.MsgFlags = gosnmp.AuthNoPriv
		} else {
			sn.MsgFlags = gosnmp.AuthPriv
		}
	} else {
		sn.Version = gosnmp.Version2c
		sn.Community = job.SNMP.Community
	}

	start := time.Now()
	if err := sn.Connect(); err != nil {
		return Result{Status: "fail", Code: "snmp_connect", Message: err.Error()}
	}
	defer sn.Conn.Close()

	oid := "1.3.6.1.2.1.1.3.0"
	resp, err := sn.Get([]string{oid})
	latency := int(time.Since(start).Milliseconds())
	if err != nil || resp == nil || len(resp.Variables) == 0 {
		if err == nil {
			err = errors.New("SNMP empty response")
		}
		return Result{Status: "fail", LatencyMs: latency, Code: "snmp_fail", Message: err.Error()}
	}

	payload := map[string]any{"oids": map[string]any{}}
	for _, v := range resp.Variables {
		payload["oids"].(map[string]any)[v.Name] = fmt.Sprintf("%v", v.Value)
	}
	return Result{Status: "ok", LatencyMs: latency, Payload: payload}
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
	scheme := getString(job.Config, "scheme", "http")
	if forceTLS {
		scheme = "https"
	}
	if v, ok := getBool(job.Config, "https"); ok && v {
		scheme = "https"
	}
	host := job.Endpoint.Endereco
	port := endpointPort(job, 0)
	path := getString(job.Config, "path", "/")
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
