package hub

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

// ServeHub inicia o listener HTTP para receber ingest de agentes em modo hub.
func ServeHub(addr string, cfg config.Config, outbox ports.OutboxRepo, log logger.Logger) {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/ingest", func(w http.ResponseWriter, r *http.Request) {
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
		for i := range batch {
			if batch[i].Meta == nil {
				batch[i].Meta = map[string]string{}
			}
			batch[i].Meta["via"] = "hub"
			batch[i].AuthHeader = auth
			_ = outbox.Append(batch[i])
		}
		log.Info("hub ingest buffered n=" + strconv.Itoa(len(batch)))
		w.WriteHeader(http.StatusAccepted)
	})

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
		cl := &http.Client{Timeout: 8 * time.Second}
		resp, err := cl.Do(req)
		if err != nil {
			http.Error(w, "upstream error", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	})

	log.Info("hub listener on " + addr)
	_ = http.ListenAndServe(addr, mux)
}
