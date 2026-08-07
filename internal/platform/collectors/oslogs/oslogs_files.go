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
	"syscall"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

type collector struct {
	prefs       func() config.CollectPrefs
	files       []string
	cursorPath  string
	batchLines  int
	maxBytes    int
	cursor      map[string]int64
	interval    time.Duration
	diag        bool
	errors      []string
	enrich      bool
	detect      bool
	include     string
	exclude     string
	minSeverity string
	local       *localReceiver
	journald    journalReader
	processors  []config.LogProcessorConfig
}

func New(cfg config.Config, prefsProvider func() config.CollectPrefs) ports.Collector {
	files := cfg.OSLogFiles
	if len(files) == 0 {
		files = defaultPaths()
	}
	return &collector{
		prefs:       prefsProvider,
		files:       files,
		cursorPath:  cfg.OSLogCursorPath,
		batchLines:  cfg.OSLogBatchLines,
		maxBytes:    cfg.OSLogMaxBytes,
		cursor:      loadCursor(cfg.OSLogCursorPath),
		interval:    cfg.OSLogInterval,
		diag:        cfg.OSLogDiag,
		enrich:      cfg.OSLogEnrich,
		detect:      cfg.OSLogDetections,
		include:     cfg.OSLogIncludeRegex,
		exclude:     cfg.OSLogExcludeRegex,
		minSeverity: cfg.OSLogMinSeverity,
		local:       newLocalReceiver(cfg.OSLogUDPAddr, cfg.OSLogTCPAddr, cfg.OSLogBatchLines, cfg.OSLogMaxBytes),
		processors:  sanitizeLogProcessors(cfg.OSLogProcessors),
		journald: newJournalReader(
			cfg.OSLogJournaldEnabled,
			cfg.OSLogJournaldUnits,
			cfg.OSLogJournaldPriorities,
			runJournalctl,
		),
	}
}

func (c *collector) Name() string { return "oslogs" }

func (c *collector) Interval() time.Duration { return c.interval }

type logEvent struct {
	SchemaVersion            int            `json:"schema_version"`
	Timestamp                string         `json:"timestamp"`
	TimestampUTC             string         `json:"timestamp_utc"`
	Source                   string         `json:"source,omitempty"`
	Host                     string         `json:"host,omitempty"`
	File                     string         `json:"file"`
	Path                     string         `json:"path,omitempty"`
	Cursor                   string         `json:"cursor,omitempty"`
	Message                  string         `json:"message"`
	App                      string         `json:"app,omitempty"`
	Service                  string         `json:"service,omitempty"`
	PID                      string         `json:"pid,omitempty"`
	Level                    string         `json:"level,omitempty"`
	Facility                 string         `json:"facility,omitempty"`
	Severity                 string         `json:"severity,omitempty"`
	Category                 string         `json:"category,omitempty"`
	Attributes               map[string]any `json:"attributes,omitempty"`
	RedactionStatus          string         `json:"redaction_status,omitempty"`
	Transport                string         `json:"transport,omitempty"`
	SourceTool               string         `json:"source_tool,omitempty"`
	SourceCategory           string         `json:"source_category,omitempty"`
	AicebergTransport        string         `json:"aiceberg_transport,omitempty"`
	AicebergToolOrigin       string         `json:"aiceberg_tool_origin,omitempty"`
	AicebergSourceCategory   string         `json:"aiceberg_source_category,omitempty"`
	AicebergSOCSourceType    string         `json:"aiceberg_soc_source_type,omitempty"`
	AicebergSOCEligible      string         `json:"aiceberg_soc_eligible,omitempty"`
	AicebergOriginConfidence string         `json:"aiceberg_origin_confidence,omitempty"`
	AicebergRouteReason      string         `json:"aiceberg_route_reason,omitempty"`
	EventCode                string         `json:"event_code,omitempty"`
	Vendor                   string         `json:"vendor,omitempty"`
	Product                  string         `json:"product,omitempty"`
	SrcIP                    string         `json:"src_ip,omitempty"`
	DstIP                    string         `json:"dst_ip,omitempty"`
	SrcHost                  string         `json:"src_host,omitempty"`
	DstHost                  string         `json:"dst_host,omitempty"`
	Username                 string         `json:"username,omitempty"`
	ProcessName              string         `json:"process_name,omitempty"`
	CommandLine              string         `json:"command_line,omitempty"`
	FileHash                 string         `json:"file_hash,omitempty"`
	Domain                   string         `json:"domain,omitempty"`
	URL                      string         `json:"url,omitempty"`
	Action                   string         `json:"action,omitempty"`
	RuleName                 string         `json:"rule_name,omitempty"`
	TechniqueID              string         `json:"technique_id,omitempty"`
	AlertID                  string         `json:"alert_id,omitempty"`
}

type payload struct {
	Events          []logEvent        `json:"events"`
	DroppedCount    int               `json:"dropped_count,omitempty"`
	LogSourceHealth []logSourceHealth `json:"log_source_health,omitempty"`
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
	if p.OSLogIncludeRegex != "" {
		c.include = p.OSLogIncludeRegex
	}
	if p.OSLogExcludeRegex != "" {
		c.exclude = p.OSLogExcludeRegex
	}
	if p.OSLogMinSeverity != "" {
		c.minSeverity = p.OSLogMinSeverity
	}
	if p.OSLogUDPAddr != "" || p.OSLogTCPAddr != "" {
		c.ensureLocalReceiver(p.OSLogUDPAddr, p.OSLogTCPAddr)
	}
	c.journald.applyPrefs(p)
	if len(p.OSLogProcessors) > 0 {
		c.processors = sanitizeLogProcessors(p.OSLogProcessors)
	}
	if len(p.OSLogFilesList) > 0 {
		c.files = p.OSLogFilesList
	}
	if p.OSLogBatchLines > 0 {
		c.batchLines = p.OSLogBatchLines
	}
	if p.OSLogMaxBytes > 0 {
		c.maxBytes = p.OSLogMaxBytes
	}

	if len(c.files) == 0 && c.local == nil && !c.journald.enabled {
		return nil, nil
	}
	hostname, _ := os.Hostname()
	c.errors = c.errors[:0]
	var events []logEvent
	var health []logSourceHealth
	droppedCount := 0
	if len(c.files) > 0 {
		for _, path := range c.files {
			evs, sourceHealth := c.readFile(path, hostname)
			sourceHealth.ReadLines = len(evs)
			for _, ev := range evs {
				processed, keep := processLogEvent(ev, c.processors)
				if !keep || shouldDropLogEvent(processed, c.include, c.exclude, c.minSeverity) {
					droppedCount++
					sourceHealth.DroppedEvents++
					sourceHealth.DropReason = "severity_or_processor"
					continue
				}
				events = append(events, processed)
				sourceHealth.AcceptedEvents++
				if len(events) >= c.batchLines {
					break
				}
			}
			sourceHealth.LastEventAt = lastEventTimestamp(evs)
			health = append(health, finalizeLogSourceHealth(sourceHealth))
			if len(events) >= c.batchLines {
				break
			}
		}
	}
	if c.journald.enabled {
		journalHealth := newLogSourceHealth("journald", "journald")
		journalEvents := c.journald.read(ctx, c.cursor, hostname, c.batchLines-len(events), c.maxBytes)
		journalHealth.PermissionStatus = "ok"
		journalHealth.LastReadAt = time.Now().UTC().Format(time.RFC3339Nano)
		journalHealth.ReadLines = len(journalEvents)
		for _, ev := range journalEvents {
			processed, keep := processLogEvent(ev, c.processors)
			if !keep || shouldDropLogEvent(processed, c.include, c.exclude, c.minSeverity) {
				droppedCount++
				journalHealth.DroppedEvents++
				journalHealth.DropReason = "severity_or_processor"
				continue
			}
			events = append(events, processed)
			journalHealth.AcceptedEvents++
			if len(events) >= c.batchLines {
				break
			}
		}
		journalHealth.LastEventAt = lastEventTimestamp(journalEvents)
		health = append(health, finalizeLogSourceHealth(journalHealth))
	}
	for _, ev := range c.readLocal(hostname) {
		processed, keep := processLogEvent(ev, c.processors)
		if !keep || shouldDropLogEvent(processed, c.include, c.exclude, c.minSeverity) {
			droppedCount++
			continue
		}
		events = append(events, processed)
		if len(events) >= c.batchLines {
			break
		}
	}
	if len(events) == 0 && droppedCount == 0 {
		if c.diag && len(c.errors) > 0 {
			return nil, formatDiagError(c.errors)
		}
		if len(health) == 0 {
			return nil, nil
		}
	}
	_ = saveCursor(c.cursorPath, c.cursor)
	return json.Marshal(payload{Events: events, DroppedCount: droppedCount, LogSourceHealth: health})
}

func (c *collector) ensureLocalReceiver(udpAddr, tcpAddr string) {
	if c.local != nil && c.local.matches(udpAddr, tcpAddr) {
		return
	}
	if c.local != nil {
		c.local.Close()
	}
	c.local = newLocalReceiver(udpAddr, tcpAddr, c.batchLines, c.maxBytes)
}

func (c *collector) readLocal(hostname string) []logEvent {
	if c.local == nil {
		return nil
	}
	items, warnings := c.local.Drain(c.batchLines)
	c.errors = append(c.errors, warnings...)
	events := make([]logEvent, 0, len(items))
	for _, item := range items {
		events = append(events, c.buildLocalEvent(hostname, item))
	}
	return events
}

func (c *collector) readFile(path, hostname string) ([]logEvent, logSourceHealth) {
	var out []logEvent
	health := newLogSourceHealth("file", path)
	f, err := os.Open(path)
	if err != nil {
		if os.IsPermission(err) {
			c.errors = append(c.errors, "permissao negada em "+path)
			health.PermissionStatus = "permission_denied"
			health.LastError = "permission_denied"
			health.Gaps = appendUniqueHealthGap(health.Gaps, "permission_denied")
		} else if os.IsNotExist(err) {
			c.errors = append(c.errors, "arquivo inexistente "+path)
			health.PermissionStatus = "missing"
			health.LastError = "file_missing"
			health.Gaps = appendUniqueHealthGap(health.Gaps, "file_missing")
		} else {
			c.errors = append(c.errors, "erro ao abrir "+path+": "+err.Error())
			health.PermissionStatus = "error"
			health.LastError = safeHealthText(err.Error(), 160)
			health.Gaps = appendUniqueHealthGap(health.Gaps, "read_error")
		}
		return out, finalizeLogSourceHealth(health)
	}
	defer f.Close()
	health.PermissionStatus = "ok"
	health.LastReadAt = time.Now().UTC().Format(time.RFC3339Nano)
	offset := c.cursor[path]
	if info, err := f.Stat(); err == nil {
		currentFileID := fileIdentity(info)
		applyFileStatHealth(&health, info, currentFileID)
		storedFileID := c.cursor[fileIdentityCursorKey(path)]
		if offset > info.Size() || (offset > 0 && storedFileID != 0 && currentFileID != 0 && storedFileID != currentFileID) {
			offset = 0
			health.Gaps = appendUniqueHealthGap(health.Gaps, "cursor_reset")
		}
		if offset > 0 && !cursorAtLineBoundary(f, offset) {
			offset = 0
			health.Gaps = appendUniqueHealthGap(health.Gaps, "cursor_not_line_boundary")
		}
		if currentFileID != 0 {
			c.cursor[fileIdentityCursorKey(path)] = currentFileID
		}
	}
	if offset > 0 {
		_, _ = f.Seek(offset, 0)
	} else {
		_, _ = f.Seek(0, 0)
	}
	r := bufio.NewReader(f)
	pending := ""
	for len(out) < c.batchLines {
		line, err := r.ReadString('\n')
		if line != "" {
			line = strings.TrimRight(line, "\r\n")
			if pending == "" || looksLikeNewLogEntry(line) {
				if pending != "" {
					out = append(out, c.buildEvent(path, hostname, pending))
					if len(out) >= c.batchLines {
						pending = ""
						break
					}
				}
				pending = line
			} else {
				pending += "\n" + line
			}
		}
		if err != nil {
			break
		}
	}
	if pending != "" && len(out) < c.batchLines {
		out = append(out, c.buildEvent(path, hostname, pending))
	}
	if pos, err := f.Seek(0, 1); err == nil {
		c.cursor[path] = pos
		health.Cursor = strconv.FormatInt(pos, 10)
	}
	return out, health
}

func fileIdentityCursorKey(path string) string {
	return path + "#file_id"
}

func fileIdentity(info os.FileInfo) int64 {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return 0
	}
	return int64(stat.Dev)<<32 ^ int64(stat.Ino)
}

func cursorAtLineBoundary(f *os.File, offset int64) bool {
	if offset <= 0 {
		return true
	}
	if _, err := f.Seek(offset-1, 0); err != nil {
		return false
	}
	buf := make([]byte, 1)
	if _, err := f.Read(buf); err != nil {
		return false
	}
	return buf[0] == '\n'
}

func (c *collector) buildEvent(path, hostname, line string) logEvent {
	app, pid, lvl, severity, facility, msg := "", "", "", "", "", line
	if c.enrich && !strings.HasPrefix(strings.TrimSpace(line), "{") {
		a, p, l, sev, fac, m := parseSyslog(line, c.maxBytes)
		if m != "" {
			msg = m
		}
		app, pid, lvl, severity, facility = a, p, l, sev, fac
	}
	if len(msg) > c.maxBytes {
		msg = msg[:c.maxBytes]
	}
	attributes := jsonAttributes(msg)
	sourceTool := sourceToolForUnix(path, attributes)
	if gelfMsg := graylogMessage(attributes); gelfMsg != "" {
		msg = gelfMsg
	}
	if lvl == "" {
		lvl = levelFromAttributes(attributes)
		severity = lvl
	}
	if lvl == "" {
		lvl = securitySignalSeverity(msg)
		severity = lvl
	}
	service := app
	if service == "" {
		service = serviceFromAttributes(attributes)
	}
	msg, redactionStatus := redactMessage(msg)
	category := ""
	if c.detect {
		category = detectUnixCategory(msg)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	cursor := strconv.FormatInt(c.cursor[path], 10)
	event := logEvent{
		SchemaVersion:   logSchemaVersion,
		Timestamp:       now,
		TimestampUTC:    now,
		Source:          hostname,
		Host:            hostname,
		File:            path,
		Path:            path,
		Cursor:          cursor,
		Message:         msg,
		App:             app,
		Service:         service,
		PID:             pid,
		Level:           lvl,
		Severity:        severity,
		Facility:        facility,
		Category:        category,
		Attributes:      attributes,
		RedactionStatus: redactionStatus,
		Transport:       "agent_file",
		SourceTool:      sourceTool,
		SourceCategory:  sourceCategoryForUnix(path, facility),
	}
	if signal, ok := parseWordPressAccessSignal(line); ok {
		return applyWordPressAccessSignal(event, signal)
	}
	return enrichSOCEvent(event)
}

func (c *collector) buildLocalEvent(hostname string, item localLogEntry) logEvent {
	msg := item.Message
	if len(msg) > c.maxBytes {
		msg = msg[:c.maxBytes]
	}
	attributes := jsonAttributes(msg)
	level := levelFromAttributes(attributes)
	msg, redactionStatus := redactMessage(msg)
	category := ""
	if c.detect {
		category = detectUnixCategory(msg)
	}
	now := item.ReceivedAt.UTC().Format(time.RFC3339Nano)
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339Nano)
	}
	sourceTool := "local_" + item.Transport
	sourcePath := "local://" + item.Transport
	return enrichSOCEvent(logEvent{
		SchemaVersion:   logSchemaVersion,
		Timestamp:       now,
		TimestampUTC:    now,
		Source:          hostname,
		Host:            hostname,
		File:            sourcePath,
		Path:            sourcePath,
		Message:         msg,
		Level:           level,
		Severity:        level,
		Category:        category,
		Attributes:      attributes,
		RedactionStatus: redactionStatus,
		Transport:       "agent_" + item.Transport,
		SourceTool:      sourceTool,
		SourceCategory:  "log",
	})
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

func levelFromAttributes(attrs map[string]any) string {
	if attrs == nil {
		return ""
	}
	for _, key := range []string{"level", "severity", "priority", "syslog_severity", "log_level"} {
		if value, ok := attrs[key]; ok {
			if normalized := normalizeSeverityValue(value); normalized != "" {
				return normalized
			}
		}
	}
	return ""
}

func normalizeSeverityValue(value any) string {
	switch v := value.(type) {
	case string:
		return normalizeSeverityString(v)
	case float64:
		return severityName(int(v))
	case int:
		return severityName(v)
	case int64:
		return severityName(int(v))
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
