package oslogs

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"time"
)

type logSourceHealth struct {
	SchemaVersion     int      `json:"schema_version"`
	SourceFingerprint string   `json:"source_fingerprint"`
	Path              string   `json:"path,omitempty"`
	Channel           string   `json:"channel,omitempty"`
	Kind              string   `json:"kind"`
	Product           string   `json:"product,omitempty"`
	Approved          bool     `json:"approved"`
	Enabled           bool     `json:"enabled"`
	LastScanAt        string   `json:"last_scan_at"`
	LastReadAt        string   `json:"last_read_at,omitempty"`
	LastEventAt       string   `json:"last_event_at,omitempty"`
	LastSentAt        string   `json:"last_sent_at,omitempty"`
	Cursor            string   `json:"cursor,omitempty"`
	FileSize          int64    `json:"file_size,omitempty"`
	FileMtime         string   `json:"file_mtime,omitempty"`
	FileIdentity      string   `json:"file_identity,omitempty"`
	ReadLines         int      `json:"read_lines"`
	AcceptedEvents    int      `json:"accepted_events"`
	DroppedEvents     int      `json:"dropped_events"`
	DropReason        string   `json:"drop_reason,omitempty"`
	LastError         string   `json:"last_error,omitempty"`
	PermissionStatus  string   `json:"permission_status"`
	Status            string   `json:"status"`
	Confidence        string   `json:"confidence"`
	Gaps              []string `json:"gaps,omitempty"`
}

func newLogSourceHealth(kind, source string) logSourceHealth {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	h := logSourceHealth{
		SchemaVersion:    1,
		Kind:             safeHealthText(kind, 40),
		Approved:         true,
		Enabled:          true,
		LastScanAt:       now,
		PermissionStatus: "unknown",
		Status:           "unknown",
		Confidence:       "low",
	}
	if strings.EqualFold(kind, "windows_eventlog") {
		h.Channel = safeHealthText(source, 180)
		h.Product = windowsHealthProduct(source)
	} else {
		h.Path = safeHealthText(source, 220)
		h.Product = unixHealthProduct(source)
	}
	h.SourceFingerprint = sourceHealthFingerprint(kind, source)
	return h
}

func finalizeLogSourceHealth(h logSourceHealth) logSourceHealth {
	if h.PermissionStatus == "" || h.PermissionStatus == "unknown" {
		h.PermissionStatus = "ok"
	}
	if h.AcceptedEvents > 0 {
		h.Status = "delivering"
		h.Confidence = "high"
		if h.LastEventAt == "" {
			h.LastEventAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		h.LastSentAt = h.LastEventAt
		return h
	}
	if h.DroppedEvents > 0 {
		h.Status = "dropped_by_severity"
		h.Confidence = "medium"
		h.Gaps = appendUniqueHealthGap(h.Gaps, "events_dropped")
		return h
	}
	if h.LastError != "" {
		switch h.PermissionStatus {
		case "permission_denied":
			h.Status = "permission_denied"
		case "missing":
			if h.Kind == "windows_eventlog" {
				h.Status = "channel_missing"
			} else {
				h.Status = "file_missing"
			}
		default:
			h.Status = "unknown"
		}
		h.Confidence = "high"
		return h
	}
	h.Status = "no_new_events"
	h.Confidence = "medium"
	return h
}

func applyFileStatHealth(h *logSourceHealth, info os.FileInfo, fileID int64) {
	h.FileSize = info.Size()
	h.FileMtime = info.ModTime().UTC().Format(time.RFC3339Nano)
	if fileID != 0 {
		h.FileIdentity = hex.EncodeToString([]byte(strconv.FormatInt(fileID, 10)))
	}
}

func sourceHealthFingerprint(kind, source string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(kind)) + "|" + strings.ToLower(strings.TrimSpace(source))))
	return hex.EncodeToString(sum[:])[:24]
}

func lastEventTimestamp(events []logEvent) string {
	for i := len(events) - 1; i >= 0; i-- {
		if strings.TrimSpace(events[i].TimestampUTC) != "" {
			return events[i].TimestampUTC
		}
		if strings.TrimSpace(events[i].Timestamp) != "" {
			return events[i].Timestamp
		}
	}
	if len(events) > 0 {
		return time.Now().UTC().Format(time.RFC3339Nano)
	}
	return ""
}

func securitySignalSeverity(message string) string {
	l := strings.ToLower(message)
	patterns := []string{
		"failed password",
		"invalid user",
		"authentication failure",
		"sasl login authentication failed",
		"sudo authentication failure",
		"pam_unix",
		"pam",
		"permission denied",
		"segfault",
		"out of memory",
		" oom",
		"nginx error",
		"nginx: [error]",
		"[error] ",
		"apache error",
		"apache2: [error]",
		"httpd: [error]",
	}
	for _, p := range patterns {
		if strings.Contains(l, p) {
			return "error"
		}
	}
	return ""
}

func appendUniqueHealthGap(gaps []string, gap string) []string {
	for _, current := range gaps {
		if current == gap {
			return gaps
		}
	}
	return append(gaps, gap)
}

func safeHealthText(value string, limit int) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\x00", "")
	if limit > 0 && len(value) > limit {
		return value[:limit]
	}
	return value
}

func unixHealthProduct(path string) string {
	l := strings.ToLower(path)
	switch {
	case strings.Contains(l, "auth"):
		return "linux_auth"
	case strings.Contains(l, "syslog"):
		return "linux_syslog"
	case strings.Contains(l, "nginx"):
		return "nginx"
	case strings.Contains(l, "apache"):
		return "apache"
	default:
		return "local_file"
	}
}

func windowsHealthProduct(channel string) string {
	l := strings.ToLower(channel)
	switch {
	case l == "security":
		return "windows_security"
	case strings.Contains(l, "sysmon"):
		return "windows_sysmon"
	case l == "system":
		return "windows_system"
	case l == "application":
		return "windows_application"
	default:
		return "windows_eventlog"
	}
}
