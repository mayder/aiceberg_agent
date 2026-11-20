package hub

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/httpx"
	"github.com/you/aiceberg_agent/internal/common/relayprobe"
)

func registerRelaySNMPProxy(mux *http.ServeMux, cfg config.Config) {
	for _, path := range []string{relayprobe.JobPath, relayprobe.ResultPath} {
		mux.HandleFunc(path, relaySNMPProxy(cfg, path))
	}
}

func relaySNMPProxy(cfg config.Config, path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if cfg.Mode() != "hub" || cfg.Agent.Token == "" {
			http.Error(w, "hub proof unavailable", http.StatusServiceUnavailable)
			return
		}
		if r.Header.Get("Authorization") == "" || r.Header.Get("X-Agent-Identity") == "" {
			http.Error(w, "relay identity required", http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 16385))
		if err != nil || len(body) > 16384 {
			http.Error(w, "payload invalid", http.StatusBadRequest)
			return
		}
		request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, cfg.APIEndpoint(path), bytes.NewReader(body))
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
		response, err := httpx.NewClient(cfg, 8*time.Second).Do(request)
		if err != nil {
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		reply, err := io.ReadAll(io.LimitReader(response.Body, 16385))
		if err != nil || len(reply) > 16384 {
			http.Error(w, "upstream response invalid", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write(reply)
	}
}
