package health

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/common/version"
)

type Snapshot struct {
	Status     string `json:"status"`
	Version    string `json:"version"`
	QueueItems int    `json:"queue_items,omitempty"`
	QueueBytes int64  `json:"queue_bytes,omitempty"`
	FlushOK    int64  `json:"flush_ok,omitempty"`
	FlushErr   int64  `json:"flush_err,omitempty"`
	CollectErr int64  `json:"collect_err,omitempty"`
}

func Serve(port int, log logger.Logger, stats func() Snapshot) {
	addr := ":" + strconv.Itoa(port)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		var snap Snapshot
		if stats != nil {
			snap = stats()
		}
		if snap.Status == "" {
			snap.Status = "ok"
		}
		if snap.Version == "" {
			snap.Version = version.Version
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snap)
	})
	log.Info("health on " + addr)
	_ = http.ListenAndServe(addr, nil)
}
