package hub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/httpx"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

// ServeHub inicia o listener HTTP para receber ingest de agentes em modo hub.
func ServeHub(addr string, cfg config.Config, outbox ports.OutboxRepo, log logger.Logger, pendingCfg *PendingConfigStore) {
	mux := http.NewServeMux()

	ingestHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "missing Authorization", http.StatusUnauthorized)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10MB
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var batch []entities.Envelope
		if err := json.Unmarshal(body, &batch); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		buffered := 0
		for i := range batch {
			if batch[i].ID == "" {
				usecase.HandleInvalidEnvelope(log, batch[i], "missing_envelope_id")
				continue
			}
			if batch[i].Meta == nil {
				batch[i].Meta = map[string]string{}
			}
			batch[i].Meta["via"] = "hub"
			if batch[i].Endpoint == "" {
				batch[i].Endpoint = r.URL.Path
			}
			batch[i].AuthHeader = auth
			if err := outbox.Append(batch[i]); err != nil {
				log.Error(logger.KV("outbox append failed",
					"event_id", batch[i].ID,
					"agent_id", batch[i].AgentID,
					"route", batch[i].Endpoint,
					"err", err,
				))
				continue
			}
			buffered++
		}
		log.Info(logger.KV("hub ingest buffered",
			"route", r.URL.Path,
			"batch_size", buffered,
		))
		if pendingCfg != nil {
			if cfgRaw, ok := pendingCfg.Pop(auth); ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status": "ok",
					"config": cfgRaw,
				})
				return
			}
		}
		w.WriteHeader(http.StatusAccepted)
	}

	mux.HandleFunc("/v1/ingest", ingestHandler)
	mux.HandleFunc("/v1/ingest/", ingestHandler)

	mux.HandleFunc("/v1/agent/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "missing Authorization", http.StatusUnauthorized)
			return
		}
		req, err := http.NewRequest(http.MethodGet, cfg.APIEndpoint("/v1/agent/config"), nil)
		if err != nil {
			http.Error(w, "upstream build error", http.StatusInternalServerError)
			return
		}
		req.Header.Set("Authorization", auth)
		cl := httpx.NewClient(cfg, 8*time.Second)
		resp, err := cl.Do(req)
		if err != nil {
			http.Error(w, "upstream error", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	})

	mux.HandleFunc("/v1/agent/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "missing Authorization", http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}
		req, err := http.NewRequest(http.MethodPost, cfg.APIEndpoint("/v1/agent/bootstrap"), bytes.NewReader(body))
		if err != nil {
			http.Error(w, "upstream build error", http.StatusInternalServerError)
			return
		}
		req.Header.Set("Authorization", auth)
		req.Header.Set("Content-Type", "application/json")
		cl := httpx.NewClient(cfg, 10*time.Second)
		resp, err := cl.Do(req)
		if err != nil {
			http.Error(w, "upstream error", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	})

	mux.HandleFunc("/v1/agent/ping", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "missing Authorization", http.StatusUnauthorized)
			return
		}
		upURL := cfg.APIEndpoint("/v1/agent/ping")
		req, err := http.NewRequest(r.Method, upURL, r.Body)
		if err != nil {
			http.Error(w, "upstream build error", http.StatusInternalServerError)
			return
		}
		req.Header.Set("Authorization", auth)
		if ct := r.Header.Get("Content-Type"); ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		cl := httpx.NewClient(cfg, 8*time.Second)
		resp, err := cl.Do(req)
		if err != nil {
			http.Error(w, "upstream error", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	})

	mux.HandleFunc("/v1/agent/update/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "missing Authorization", http.StatusUnauthorized)
			return
		}

		rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
		if rawURL == "" {
			http.Error(w, "missing url", http.StatusBadRequest)
			return
		}
		targetURL, err := neturl.Parse(rawURL)
		if err != nil || targetURL == nil || !targetURL.IsAbs() {
			http.Error(w, "invalid url", http.StatusBadRequest)
			return
		}
		if targetURL.Scheme != "http" && targetURL.Scheme != "https" {
			http.Error(w, "unsupported url scheme", http.StatusBadRequest)
			return
		}

		upReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, targetURL.String(), nil)
		if err != nil {
			http.Error(w, "upstream build error", http.StatusInternalServerError)
			return
		}

		useAgentAuth := strings.EqualFold(r.URL.Query().Get("use_agent_auth"), "1") ||
			strings.EqualFold(r.URL.Query().Get("use_agent_auth"), "true")
		if useAgentAuth && sameHost(targetURL.String(), cfg.APIBaseURL) {
			upReq.Header.Set("Authorization", auth)
		}

		timeout := cfg.AutoUpdateTimeout
		if timeout <= 0 {
			timeout = 5 * time.Minute
		}
		cl := httpx.NewClient(cfg, timeout)
		resp, err := cl.Do(upReq)
		if err != nil {
			http.Error(w, "upstream error", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		if ct := resp.Header.Get("Content-Type"); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		if cd := resp.Header.Get("Content-Disposition"); cd != "" {
			w.Header().Set("Content-Disposition", cd)
		}
		if clh := resp.Header.Get("Content-Length"); clh != "" {
			w.Header().Set("Content-Length", clh)
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	})

	log.Info(logger.KV("hub listener on",
		"addr", addr,
	))
	_ = http.ListenAndServe(addr, mux)
}

func sameHost(rawA, rawB string) bool {
	a, errA := neturl.Parse(strings.TrimSpace(rawA))
	b, errB := neturl.Parse(strings.TrimSpace(rawB))
	if errA != nil || errB != nil || a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.Host, b.Host)
}
