package hub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/httpx"
	"github.com/you/aiceberg_agent/internal/common/relayprobe"
)

func registerRelaySNMPProxy(mux *http.ServeMux, cfg config.Config) {
	client := httpx.NewClient(cfg, 8*time.Second)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	for _, path := range []string{relayprobe.JobPath, relayprobe.ResultPath} {
		mux.HandleFunc(path, relaySNMPProxy(cfg, path, client))
	}
	if cfg.RelaySNMPAssignmentEnabled() {
		for _, path := range []string{relayprobe.AssignmentJobPath, relayprobe.AssignmentResultPath} {
			mux.HandleFunc(path, relaySNMPProxy(cfg, path, client))
		}
	}
}

func relaySNMPProxy(cfg config.Config, path string, client *http.Client) http.HandlerFunc {
	assignment := relayprobe.IsAssignmentPath(path)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if cfg.Mode() != "hub" || cfg.Agent.Token == "" {
			http.Error(w, "hub proof unavailable", http.StatusServiceUnavailable)
			return
		}
		if assignment && r.TLS == nil {
			http.Error(w, "verified relay transport required", http.StatusForbidden)
			return
		}
		if assignment && (r.URL.RawQuery != "" || r.URL.ForceQuery) {
			http.Error(w, "assignment query parameters forbidden", http.StatusForbidden)
			return
		}
		if r.Header.Get("Authorization") == "" || r.Header.Get("X-Agent-Identity") == "" {
			http.Error(w, "relay identity required", http.StatusUnauthorized)
			return
		}
		limit := relayprobe.BodyLimit(path)
		body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
		if err != nil || int64(len(body)) > limit {
			http.Error(w, "payload invalid", http.StatusBadRequest)
			return
		}
		upstream := cfg.APIEndpoint(path)
		if (assignment || (path == relayprobe.JobPath && relayUsesCredentialReference(body))) && !httpx.VerifiedHTTPS(upstream, cfg.TLSInsecureSkip) {
			http.Error(w, "verified upstream required", http.StatusServiceUnavailable)
			return
		}
		request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, upstream, bytes.NewReader(body))
		if err != nil {
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
			return
		}
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		request.Header.Set("Authorization", r.Header.Get("Authorization"))
		request.Header.Set("X-Agent-Identity", r.Header.Get("X-Agent-Identity"))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Aiceberg-Hub-Time", timestamp)
		request.Header.Set("X-Aiceberg-Hub-Proof", relayprobe.Proof(cfg.Agent.Token, r.Header.Get("Authorization"), path, body, timestamp))
		response, err := client.Do(request)
		if err != nil {
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		reply, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
		if err != nil || int64(len(reply)) > limit {
			http.Error(w, "upstream response invalid", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store, private")
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write(reply)
	}
}

func relayUsesCredentialReference(body []byte) bool {
	var control map[string]any
	return json.Unmarshal(body, &control) == nil &&
		(control["operation"] == "resolve_snmp_profile" || control["credential_mode"] == "reference-v1")
}
