//go:build !windows
// +build !windows

package oslogs

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

type collector struct {
	prefs      func() config.CollectPrefs
	files      []string
	cursorPath string
	batchLines int
	maxBytes   int
	cursor     map[string]int64
	interval   time.Duration
	diag       bool
	errors     []string
	enrich     bool
	detect     bool
}

func New(cfg config.Config, prefsProvider func() config.CollectPrefs) ports.Collector {
	files := cfg.OSLogFiles
	if len(files) == 0 {
		files = defaultPaths()
	}
	return &collector{
		prefs:      prefsProvider,
		files:      files,
		cursorPath: cfg.OSLogCursorPath,
		batchLines: cfg.OSLogBatchLines,
		maxBytes:   cfg.OSLogMaxBytes,
		cursor:     loadCursor(cfg.OSLogCursorPath),
		interval:   cfg.OSLogInterval,
		diag:       cfg.OSLogDiag,
		enrich:     cfg.OSLogEnrich,
		detect:     cfg.OSLogDetections,
	}
}

func (c *collector) Name() string { return "oslogs" }

func (c *collector) Interval() time.Duration { return c.interval }

type logEvent struct {
	Timestamp string `json:"timestamp"`
	Source    string `json:"source,omitempty"`
	File      string `json:"file"`
	Message   string `json:"message"`
	App       string `json:"app,omitempty"`
	PID       string `json:"pid,omitempty"`
	Level     string `json:"level,omitempty"`
	Facility  string `json:"facility,omitempty"`
	Severity  string `json:"severity,omitempty"`
	Category  string `json:"category,omitempty"`
}

type payload struct {
	Events []logEvent `json:"events"`
}

func (c *collector) Collect(ctx context.Context) ([]byte, error) {
	p := config.CollectPrefs{}
	if c.prefs != nil {
		p = c.prefs()
	}
	// Se logs SOC estiverem desabilitados via prefs, retorne.
	if !p.OSLogFiles {
		return nil, nil
	}
	// Ajuste de flags dinâmicos
	c.diag = p.OSLogDiag
	c.enrich = p.OSLogEnrich
	c.detect = p.OSLogDetections
	if len(p.OSLogFilesList) > 0 {
		c.files = p.OSLogFilesList
	}
	if p.OSLogBatchLines > 0 {
		c.batchLines = p.OSLogBatchLines
	}
	if p.OSLogMaxBytes > 0 {
		c.maxBytes = p.OSLogMaxBytes
	}

	if len(c.files) == 0 {
		return nil, nil
	}
	hostname, _ := os.Hostname()
	c.errors = c.errors[:0]
	var events []logEvent
	for _, path := range c.files {
		evs := c.readFile(path, hostname)
		events = append(events, evs...)
		if len(events) >= c.batchLines {
			break
		}
	}
	if len(events) == 0 {
		if c.diag && len(c.errors) > 0 {
			return nil, formatDiagError(c.errors)
		}
		if c.diag {
			return nil, formatDiagError([]string{"nenhum evento lido; verifique OSLOG_FILES, existência e permissões"})
		}
		return nil, nil
	}
	_ = saveCursor(c.cursorPath, c.cursor)
	return json.Marshal(payload{Events: events})
}

func (c *collector) readFile(path, hostname string) []logEvent {
	var out []logEvent
	f, err := os.Open(path)
	if err != nil {
		if os.IsPermission(err) {
			c.errors = append(c.errors, "permissao negada em "+path)
		} else if os.IsNotExist(err) {
			c.errors = append(c.errors, "arquivo inexistente "+path)
		} else {
			c.errors = append(c.errors, "erro ao abrir "+path+": "+err.Error())
		}
		return out
	}
	defer f.Close()
	offset := c.cursor[path]
	if offset > 0 {
		_, _ = f.Seek(offset, 0)
	}
	r := bufio.NewReader(f)
	for len(out) < c.batchLines {
		line, err := r.ReadString('\n')
		if line != "" {
			line = strings.TrimRight(line, "\r\n")
			app, pid, lvl, severity, facility, msg := "", "", "", "", "", line
			if c.enrich {
				a, p, l, sev, fac, m := parseSyslog(line, c.maxBytes)
				if m != "" {
					msg = m
				}
				app, pid, lvl, severity, facility = a, p, l, sev, fac
			}
			if len(msg) > c.maxBytes {
				msg = msg[:c.maxBytes]
			}
			category := ""
			if c.detect {
				category = detectUnixCategory(msg)
			}
			out = append(out, logEvent{
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				Source:    hostname,
				File:      path,
				Message:   msg,
				App:       app,
				PID:       pid,
				Level:     lvl,
				Severity:  severity,
				Facility:  facility,
				Category:  category,
			})
		}
		if err != nil {
			break
		}
	}
	if pos, err := f.Seek(0, 1); err == nil {
		c.cursor[path] = pos
	}
	return out
}

func defaultPaths() []string {
	if runtime.GOOS == "darwin" {
		return []string{"/var/log/system.log", "/var/log/install.log"}
	}
	// fallback para Linux e demais Unix-like
	return []string{"/var/log/auth.log", "/var/log/syslog", "/var/log/messages"}
}

func loadCursor(path string) map[string]int64 {
	b, err := os.ReadFile(path)
	if err != nil {
		return map[string]int64{}
	}
	var cur map[string]int64
	if err := json.Unmarshal(b, &cur); err != nil {
		return map[string]int64{}
	}
	return cur
}

func saveCursor(path string, cur map[string]int64) error {
	if path == "" {
		return nil
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	raw, _ := json.Marshal(cur)
	return os.WriteFile(path, raw, 0o600)
}

func formatDiagError(errs []string) error {
	return fmt.Errorf("oslogs: %s", strings.Join(errs, "; "))
}

// parseSyslog tenta extrair app, pid, level e mensagem de uma linha estilo syslog.
func parseSyslog(line string, maxBytes int) (app, pid, level, severity, facility, msg string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	msg = line
	if strings.HasPrefix(msg, "<") {
		if end := strings.Index(msg, ">"); end > 1 {
			if pri, err := strconv.Atoi(msg[1:end]); err == nil {
				severity = severityName(pri & 7)
				facility = facilityName(pri >> 3)
			}
			msg = strings.TrimSpace(msg[end+1:])
		}
	}
	if len(msg) > maxBytes {
		msg = msg[:maxBytes]
	}
	if idx := strings.Index(msg, ":"); idx >= 0 {
		header := strings.TrimSpace(msg[:idx])
		msg = strings.TrimSpace(msg[idx+1:])
		fields := strings.Fields(header)
		if len(fields) > 0 {
			appPid := fields[len(fields)-1]
			app, pid = splitAppPid(appPid)
		}
	}
	level = severity
	if level == "" {
		level = detectLevel(msg)
	}
	return
}

func splitAppPid(s string) (app, pid string) {
	if i := strings.Index(s, "["); i >= 0 {
		app = s[:i]
		if j := strings.Index(s[i:], "]"); j > 0 {
			pid = s[i+1 : i+j]
		}
		return app, pid
	}
	return s, ""
}

func detectLevel(msg string) string {
	l := strings.ToLower(strings.TrimSpace(msg))
	switch {
	case strings.HasPrefix(l, "err"), strings.HasPrefix(l, "fail"):
		return "error"
	case strings.HasPrefix(l, "warn"):
		return "warning"
	case strings.HasPrefix(l, "crit"), strings.HasPrefix(l, "fatal"), strings.HasPrefix(l, "panic"):
		return "critical"
	case strings.HasPrefix(l, "info"):
		return "info"
	case strings.HasPrefix(l, "debug"):
		return "debug"
	default:
		return ""
	}
}

func severityName(code int) string {
	switch code {
	case 0:
		return "emergency"
	case 1:
		return "alert"
	case 2:
		return "critical"
	case 3:
		return "error"
	case 4:
		return "warning"
	case 5:
		return "notice"
	case 6:
		return "info"
	case 7:
		return "debug"
	default:
		return ""
	}
}

func facilityName(code int) string {
	switch code {
	case 0:
		return "kernel"
	case 1:
		return "user"
	case 2:
		return "mail"
	case 3:
		return "daemon"
	case 4:
		return "auth"
	case 5:
		return "syslog"
	case 6:
		return "lpr"
	case 7:
		return "news"
	case 8:
		return "uucp"
	case 9:
		return "cron"
	case 10:
		return "authpriv"
	case 11:
		return "ftp"
	default:
		return ""
	}
}

func detectUnixCategory(msg string) string {
	l := strings.ToLower(msg)
	switch {
	case strings.Contains(l, "failed password") || strings.Contains(l, "authentication failure") || strings.Contains(l, "invalid user"):
		return "auth_fail"
	case strings.Contains(l, "session opened for user") || strings.Contains(l, "accepted password"):
		return "auth_success"
	case strings.Contains(l, "sudo:") && strings.Contains(l, "authentication failure"):
		return "sudo_fail"
	case strings.Contains(l, "sudo:") && strings.Contains(l, "tty="):
		return "sudo_use"
	case strings.Contains(l, "pam_unix"):
		return "pam_event"
	case strings.Contains(l, "segfault"):
		return "crash"
	case strings.Contains(l, "oom-killer") || strings.Contains(l, "out of memory"):
		return "oom"
	case strings.Contains(l, "service:") && strings.Contains(l, "state"):
		return "service_state"
	default:
		return ""
	}
}
