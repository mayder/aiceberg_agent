package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/you/aiceberg_agent/internal/data/local/prefs"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

type state struct {
	mu            sync.Mutex
	hits          map[string]int
	ingested      map[string]int
	ingestedAuth  map[string]int
	pingGet       int
	pingPost      int
	bootstraps    int
	configGets    int
	configServed  bool
	agentlessJobs int
	agentlessObs  int
}

func newState() *state {
	return &state{
		hits:         make(map[string]int),
		ingested:     make(map[string]int),
		ingestedAuth: make(map[string]int),
	}
}

func (s *state) hit(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hits[path]++
}

func (s *state) addIngest(path string, auth string, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hits[path]++
	s.ingested[path] += n
	if auth != "" {
		s.ingestedAuth[auth] += n
	}
}

func (s *state) incPingGet() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pingGet++
}

func (s *state) incPingPost() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pingPost++
}

func (s *state) incBootstrap() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bootstraps++
}

func (s *state) incConfigGet() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.configGets++
	if s.configServed {
		return false
	}
	s.configServed = true
	return true
}

func (s *state) addAgentlessJobs(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentlessJobs += n
}

func (s *state) addAgentlessObs(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentlessObs += n
}

func main() {
	port := envInt("E2E_BACKEND_PORT", 8082)
	addr := ":" + strconv.Itoa(port)
	cfgMode := strings.ToLower(os.Getenv("E2E_CONFIG_MODE"))
	st := newState()

	mux := http.NewServeMux()
	mux.HandleFunc("/__stats", func(w http.ResponseWriter, r *http.Request) {
		st.mu.Lock()
		defer st.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"hits":        st.hits,
			"ingested":    st.ingested,
			"by_auth":     st.ingestedAuth,
			"ping_get":    st.pingGet,
			"ping_post":   st.pingPost,
			"bootstraps":  st.bootstraps,
			"config_gets": st.configGets,
			"agentless": map[string]int{
				"jobs": st.agentlessJobs,
				"obs":  st.agentlessObs,
			},
		})
	})

	mux.HandleFunc("/v1/agent/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		st.hit(r.URL.Path)
		servePayload := cfgMode == "payload" && st.incConfigGet()
		if !servePayload {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		def := prefs.Default()
		collectNow := []string{"health"}
		payload := usecase.ConfigPayload{
			Version:    "e2e-1",
			Collect:    def,
			CollectNow: &collectNow,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})

	mux.HandleFunc("/v1/agent/ping", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			st.incPingGet()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"challenge": "e2e"})
		case http.MethodPost:
			st.incPingPost()
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/v1/agent/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		st.incBootstrap()
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/v1/hub-agentless/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		st.hit(r.URL.Path)
		jobs := []map[string]any{
			{
				"check_id": 101,
				"tipo":     "tcp",
				"endpoint": map[string]any{
					"tipo":     "ip",
					"endereco": "127.0.0.1",
					"porta":    80,
				},
			},
		}
		st.addAgentlessJobs(len(jobs))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"count":  len(jobs),
			"jobs":   jobs,
		})
	})

	mux.HandleFunc("/v1/hub-agentless/observations", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}
		_ = r.Body.Close()
		var payload struct {
			Observations []json.RawMessage `json:"observations"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		st.addAgentlessObs(len(payload.Observations))
		w.WriteHeader(http.StatusOK)
	})

	ingestHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}
		_ = r.Body.Close()
		var batch []json.RawMessage
		if err := json.Unmarshal(body, &batch); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		st.addIngest(r.URL.Path, r.Header.Get("Authorization"), len(batch))
		w.WriteHeader(http.StatusAccepted)
	}
	mux.HandleFunc("/v1/ingest", ingestHandler)
	mux.HandleFunc("/v1/ingest/", ingestHandler)
	mux.HandleFunc("/v1/logs/raw", ingestHandler)

	log.Printf("[e2e-backend] listening on %s\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("[e2e-backend] server error: %v\n", err)
		os.Exit(1)
	}
}

func envInt(key string, def int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
