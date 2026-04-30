package health

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/common/version"
)

type Snapshot struct {
	Status           string  `json:"status"`
	Version          string  `json:"version"`
	QueueItems       int     `json:"queue_items,omitempty"`
	QueueBytes       int64   `json:"queue_bytes,omitempty"`
	FlushOK          int64   `json:"flush_ok,omitempty"`
	FlushErr         int64   `json:"flush_err,omitempty"`
	CollectErr       int64   `json:"collect_err,omitempty"`
	InvalidEnv       int64   `json:"invalid_envelopes,omitempty"`
	AgentlessJobs    int64   `json:"agentless_jobs,omitempty"`
	UptimeSec        int64   `json:"uptime_sec,omitempty"`
	ProcRSS          int64   `json:"proc_rss_bytes,omitempty"`
	ProcCPU          float64 `json:"proc_cpu_percent,omitempty"`
	Goroutines       int     `json:"goroutines,omitempty"`
	LastCollectMs    int64   `json:"last_collect_ms,omitempty"`
	LastFlushMs      int64   `json:"last_flush_ms,omitempty"`
	LastFlushBatch   int64   `json:"last_flush_batch,omitempty"`
	IngestTimeoutSec int64   `json:"ingest_timeout_sec,omitempty"`
	FlushIntervalSec int64   `json:"flush_interval_sec,omitempty"`
	FlushBatchLimit  int     `json:"flush_batch_limit,omitempty"`
	FlushDetail      any     `json:"flush_detail,omitempty"`
	Channel          any     `json:"channel,omitempty"`
}

func Serve(port int, log logger.Logger, stats func() Snapshot) {
	addr := ":" + strconv.Itoa(port)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		var snap Snapshot
		if stats != nil {
			snap = stats()
		}
		encodeHealthSnapshot(w, snap)
	})
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
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
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		writeMetric(w, "agent_queue_items", float64(snap.QueueItems))
		writeMetric(w, "agent_queue_bytes", float64(snap.QueueBytes))
		writeMetric(w, "agent_flush_ok_total", float64(snap.FlushOK))
		writeMetric(w, "agent_flush_err_total", float64(snap.FlushErr))
		writeMetric(w, "agent_collect_err_total", float64(snap.CollectErr))
		writeMetric(w, "agent_invalid_envelopes_total", float64(snap.InvalidEnv))
		writeMetric(w, "agent_agentless_jobs_total", float64(snap.AgentlessJobs))
		writeMetric(w, "agent_uptime_seconds", float64(snap.UptimeSec))
		writeMetric(w, "agent_proc_rss_bytes", float64(snap.ProcRSS))
		writeMetric(w, "agent_proc_cpu_percent", snap.ProcCPU)
		writeMetric(w, "agent_goroutines", float64(snap.Goroutines))
		writeMetric(w, "agent_last_collect_ms", float64(snap.LastCollectMs))
		writeMetric(w, "agent_last_flush_ms", float64(snap.LastFlushMs))
		writeMetric(w, "agent_last_flush_batch", float64(snap.LastFlushBatch))
		writeMetric(w, "agent_ingest_timeout_seconds", float64(snap.IngestTimeoutSec))
		writeMetric(w, "agent_flush_interval_seconds", float64(snap.FlushIntervalSec))
		writeMetric(w, "agent_flush_batch_limit", float64(snap.FlushBatchLimit))
		writeLabelMetric(w, "agent_info", 1, map[string]string{"version": snap.Version, "status": snap.Status})
	})
	log.Info(logger.KV("health on",
		"addr", addr,
	))
	_ = http.ListenAndServe(addr, nil)
}

func encodeHealthSnapshot(w http.ResponseWriter, snap Snapshot) {
	if snap.Status == "" {
		snap.Status = "ok"
	}
	if snap.Version == "" {
		snap.Version = version.Version
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snap)
}

func writeMetric(w http.ResponseWriter, name string, val float64) {
	_, _ = w.Write([]byte(name + " " + strconv.FormatFloat(val, 'f', -1, 64) + "\n"))
}

func writeLabelMetric(w http.ResponseWriter, name string, val float64, labels map[string]string) {
	_, _ = w.Write([]byte(name + formatLabels(labels) + " " + strconv.FormatFloat(val, 'f', -1, 64) + "\n"))
}

func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	out := "{"
	first := true
	for k, v := range labels {
		if !first {
			out += ","
		}
		first = false
		out += k + "=\"" + v + "\""
	}
	out += "}"
	return out
}
