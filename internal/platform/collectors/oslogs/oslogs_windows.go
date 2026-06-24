//go:build windows
// +build windows

package oslogs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

type winCollector struct {
	prefs       func() config.CollectPrefs
	channels    []string
	providers   []string
	eventIDs    []uint64
	cursorPath  string
	cursor      map[string]uint64
	batchLines  int
	maxBytes    int
	interval    time.Duration
	diag        bool
	errors      []string
	detect      bool
	include     string
	exclude     string
	minSeverity string
	processors  []config.LogProcessorConfig
}

type logEvent struct {
	SchemaVersion            int            `json:"schema_version"`
	Timestamp                string         `json:"timestamp"`
	TimestampUTC             string         `json:"timestamp_utc"`
	Source                   string         `json:"source,omitempty"`
	Host                     string         `json:"host,omitempty"`
	Channel                  string         `json:"channel,omitempty"`
	Path                     string         `json:"path,omitempty"`
	Cursor                   string         `json:"cursor,omitempty"`
	EventID                  uint64         `json:"event_id,omitempty"`
	RecordID                 uint64         `json:"record_id,omitempty"`
	Provider                 string         `json:"provider,omitempty"`
	Level                    string         `json:"level,omitempty"`
	Severity                 string         `json:"severity,omitempty"`
	Computer                 string         `json:"computer,omitempty"`
	Message                  string         `json:"message"`
	Category                 string         `json:"category,omitempty"`
	Service                  string         `json:"service,omitempty"`
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

func New(cfg config.Config, prefsProvider func() config.CollectPrefs) ports.Collector {
	ch := cfg.OSLogWinChannels
	if len(ch) == 0 {
		ch = []string{"Security", "System", "Application", "Microsoft-Windows-Sysmon/Operational"}
	}
	return &winCollector{
		prefs:       prefsProvider,
		channels:    ch,
		providers:   sanitizeWindowsProviders(cfg.OSLogWinProviders),
		eventIDs:    sanitizeWindowsEventIDs(cfg.OSLogWinEventIDs),
		cursorPath:  cfg.OSLogCursorPath,
		cursor:      loadCursorWin(cfg.OSLogCursorPath),
		batchLines:  cfg.OSLogBatchLines,
		maxBytes:    cfg.OSLogMaxBytes,
		interval:    cfg.OSLogInterval,
		diag:        cfg.OSLogDiag,
		detect:      cfg.OSLogDetections,
		include:     cfg.OSLogIncludeRegex,
		exclude:     cfg.OSLogExcludeRegex,
		minSeverity: cfg.OSLogMinSeverity,
		processors:  sanitizeLogProcessors(cfg.OSLogProcessors),
	}
}

func (c *winCollector) Name() string { return "oslogs" }

func (c *winCollector) Interval() time.Duration { return c.interval }

func (c *winCollector) Collect(ctx context.Context) ([]byte, error) {
	p := config.CollectPrefs{}
	if c.prefs != nil {
		p = c.prefs()
	}
	if !p.OSLogWinChannels {
		return nil, nil
	}
	c.diag = p.OSLogDiag
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
	if len(p.OSLogWinChList) > 0 {
		c.channels = p.OSLogWinChList
	}
	if len(p.OSLogWinProviders) > 0 {
		c.providers = sanitizeWindowsProviders(p.OSLogWinProviders)
	}
	if len(p.OSLogWinEventIDs) > 0 {
		c.eventIDs = sanitizeWindowsEventIDs(p.OSLogWinEventIDs)
	}
	if len(p.OSLogProcessors) > 0 {
		c.processors = sanitizeLogProcessors(p.OSLogProcessors)
	}
	if p.OSLogBatchLines > 0 {
		c.batchLines = p.OSLogBatchLines
	}
	if p.OSLogMaxBytes > 0 {
		c.maxBytes = p.OSLogMaxBytes
	}

	hostname, _ := os.Hostname()
	c.errors = c.errors[:0]
	var out []logEvent
	var health []logSourceHealth
	droppedCount := 0

	for _, ch := range c.channels {
		if len(out) >= c.batchLines {
			break
		}
		sourceHealth := newLogSourceHealth("windows_eventlog", ch)
		sourceHealth.PermissionStatus = "ok"
		sourceHealth.LastReadAt = time.Now().UTC().Format(time.RFC3339Nano)
		errorsBefore := len(c.errors)
		last := c.cursor[ch]
		events := c.fetchChannel(ctx, ch, last, c.batchLines-len(out), hostname)
		sourceHealth.ReadLines = len(events)
		sourceHealth.Cursor = strconv.FormatUint(last, 10)
		if len(c.errors) > errorsBefore {
			sourceHealth.PermissionStatus = "missing"
			sourceHealth.LastError = safeHealthText(c.errors[len(c.errors)-1], 160)
			sourceHealth.Gaps = appendUniqueHealthGap(sourceHealth.Gaps, "channel_or_permission_error")
		}
		if len(events) > 0 {
			if c.detect {
				for i := range events {
					events[i].Category = windowsCategory(events[i].EventID, events[i].Message)
					if events[i].Level == "" && events[i].Category == "auth_fail" {
						events[i].Level = "error"
						events[i].Severity = "error"
					}
				}
			}
			for _, ev := range events {
				processed, keep := processLogEvent(ev, c.processors)
				if !keep || shouldDropLogEvent(processed, c.include, c.exclude, c.minSeverity) {
					droppedCount++
					sourceHealth.DroppedEvents++
					sourceHealth.DropReason = "severity_or_processor"
					continue
				}
				out = append(out, processed)
				sourceHealth.AcceptedEvents++
				if len(out) >= c.batchLines {
					break
				}
			}
			maxRec := last
			for _, ev := range events {
				if ev.RecordID > maxRec {
					maxRec = ev.RecordID
				}
			}
			c.cursor[ch] = maxRec
			sourceHealth.Cursor = strconv.FormatUint(maxRec, 10)
		}
		sourceHealth.LastEventAt = lastEventTimestamp(events)
		health = append(health, finalizeLogSourceHealth(sourceHealth))
	}

	if len(out) == 0 && droppedCount == 0 {
		if c.diag && len(c.errors) > 0 {
			return nil, formatDiagError(c.errors)
		}
		if len(health) == 0 {
			return nil, nil
		}
	}
	_ = saveCursorWin(c.cursorPath, c.cursor)
	return json.Marshal(payload{Events: out, DroppedCount: droppedCount, LogSourceHealth: health})
}

func (c *winCollector) fetchChannel(ctx context.Context, channel string, lastRecord uint64, limit int, hostname string) []logEvent {
	var events []logEvent
	query := windowsEventQuery(lastRecord, c.providers, c.eventIDs, c.minSeverity)
	args := []string{"qe", channel, "/q:" + query, "/f:XML", "/c:" + strconv.Itoa(limit), "/rd:true"}
	cmd := exec.CommandContext(ctx, "wevtutil", args...)
	raw, err := cmd.Output()
	if err != nil {
		c.errors = append(c.errors, "falha wevtutil "+channel+": "+err.Error())
		return events
	}
	blocks := splitEventXML(raw)
	for _, blk := range blocks {
		ev := parseEventXMLBlock(blk, channel, hostname, c.maxBytes)
		if ev.RecordID == 0 {
			ev = parseEventBlock(string(blk), channel, hostname, c.maxBytes)
		}
		if ev.RecordID == 0 {
			continue
		}
		if !windowsEventMatches(ev, c.providers, c.eventIDs) {
			continue
		}
		events = append(events, ev)
	}
	return events
}

type winEventXML struct {
	System struct {
		Provider struct {
			Name string `xml:"Name,attr"`
		} `xml:"Provider"`
		EventID     uint64 `xml:"EventID"`
		Level       int    `xml:"Level"`
		TimeCreated struct {
			SystemTime string `xml:"SystemTime,attr"`
		} `xml:"TimeCreated"`
		EventRecordID uint64 `xml:"EventRecordID"`
		Channel       string `xml:"Channel"`
		Computer      string `xml:"Computer"`
	} `xml:"System"`
	EventData struct {
		Data []string `xml:"Data"`
	} `xml:"EventData"`
}

func splitEventXML(raw []byte) [][]byte {
	var out [][]byte
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "Event" {
			continue
		}
		var buf bytes.Buffer
		enc := xml.NewEncoder(&buf)
		_ = enc.EncodeToken(start)
		depth := 1
		for depth > 0 {
			tok, err = decoder.Token()
			if err != nil {
				break
			}
			switch t := tok.(type) {
			case xml.StartElement:
				depth++
				_ = enc.EncodeToken(t)
			case xml.EndElement:
				depth--
				_ = enc.EncodeToken(t)
			default:
				_ = enc.EncodeToken(tok)
			}
		}
		_ = enc.Flush()
		if buf.Len() > 0 {
			out = append(out, append([]byte(nil), buf.Bytes()...))
		}
	}
	return out
}

func parseEventXMLBlock(block []byte, channel, hostname string, maxBytes int) logEvent {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	ev := logEvent{
		SchemaVersion:  logSchemaVersion,
		Channel:        channel,
		Path:           channel,
		Source:         hostname,
		Host:           hostname,
		Timestamp:      now,
		TimestampUTC:   now,
		Transport:      "agent_windows_eventlog",
		SourceTool:     "windows_eventlog",
		SourceCategory: sourceCategoryForWindows(channel),
	}
	var parsed winEventXML
	if err := xml.Unmarshal(block, &parsed); err != nil {
		return ev
	}
	ev.EventID = parsed.System.EventID
	ev.RecordID = parsed.System.EventRecordID
	ev.Provider = strings.TrimSpace(parsed.System.Provider.Name)
	if parsed.System.Channel != "" {
		ev.Channel = parsed.System.Channel
		ev.Path = parsed.System.Channel
		ev.SourceCategory = sourceCategoryForWindows(parsed.System.Channel)
	}
	if parsed.System.Computer != "" {
		ev.Computer = parsed.System.Computer
	}
	if parsed.System.TimeCreated.SystemTime != "" {
		ev.Timestamp = parsed.System.TimeCreated.SystemTime
		ev.TimestampUTC = parsed.System.TimeCreated.SystemTime
	}
	ev.Level = windowsLevelName(parsed.System.Level)
	ev.Severity = ev.Level
	msg := strings.TrimSpace(strings.Join(parsed.EventData.Data, "\n"))
	if msg == "" {
		msg = string(block)
	}
	if len(msg) > maxBytes {
		msg = msg[:maxBytes]
	}
	attributes := jsonAttributes(msg)
	msg, redactionStatus := redactMessage(msg)
	ev.Message = msg
	ev.RedactionStatus = redactionStatus
	ev.Attributes = attributes
	if ev.Provider != "" {
		if ev.Attributes == nil {
			ev.Attributes = map[string]any{}
		}
		ev.Attributes["provider"] = ev.Provider
	}
	ev.Cursor = strconv.FormatUint(ev.RecordID, 10)
	return enrichSOCEvent(ev)
}

func windowsLevelName(level int) string {
	switch level {
	case 1:
		return "critical"
	case 2:
		return "error"
	case 3:
		return "warning"
	case 4:
		return "info"
	case 5:
		return "debug"
	default:
		return ""
	}
}

func splitEvents(s string) []string {
	var out []string
	scanner := bufio.NewScanner(strings.NewReader(s))
	var buf bytes.Buffer
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Event[") {
			if buf.Len() > 0 {
				out = append(out, buf.String())
				buf.Reset()
			}
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	if buf.Len() > 0 {
		out = append(out, buf.String())
	}
	return out
}

func parseEventBlock(block, channel, hostname string, maxBytes int) logEvent {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	ev := logEvent{
		SchemaVersion:  logSchemaVersion,
		Channel:        channel,
		Path:           channel,
		Source:         hostname,
		Host:           hostname,
		Timestamp:      now,
		TimestampUTC:   now,
		Transport:      "agent_windows_eventlog",
		SourceTool:     "windows_eventlog",
		SourceCategory: sourceCategoryForWindows(channel),
	}
	lines := strings.Split(block, "\n")
	for _, ln := range lines {
		ln = strings.TrimSpace(strings.ReplaceAll(ln, "\x00", ""))
		if strings.HasPrefix(ln, "Event ID:") {
			if id, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(ln, "Event ID:")), 10, 64); err == nil {
				ev.EventID = id
			}
		} else if strings.HasPrefix(ln, "Record ID:") {
			if id, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(ln, "Record ID:")), 10, 64); err == nil {
				ev.RecordID = id
			}
		} else if strings.HasPrefix(ln, "Provider Name:") {
			ev.Provider = strings.TrimSpace(strings.TrimPrefix(ln, "Provider Name:"))
		} else if strings.HasPrefix(ln, "Level:") {
			rawLevel := strings.TrimSpace(strings.TrimPrefix(ln, "Level:"))
			if normalized := normalizeSeverityString(rawLevel); normalized != "" {
				ev.Level = normalized
			} else {
				ev.Level = rawLevel
			}
			ev.Severity = ev.Level
		} else if strings.HasPrefix(ln, "Computer:") {
			ev.Computer = strings.TrimSpace(strings.TrimPrefix(ln, "Computer:"))
		}
	}
	msg := block
	if len(msg) > maxBytes {
		msg = msg[:maxBytes]
	}
	attributes := jsonAttributes(msg)
	msg, redactionStatus := redactMessage(msg)
	ev.Message = msg
	ev.RedactionStatus = redactionStatus
	ev.Attributes = attributes
	if ev.Provider != "" {
		if ev.Attributes == nil {
			ev.Attributes = map[string]any{}
		}
		ev.Attributes["provider"] = ev.Provider
	}
	ev.Cursor = strconv.FormatUint(ev.RecordID, 10)
	return enrichSOCEvent(ev)
}

func windowsEventQuery(lastRecord uint64, providers []string, eventIDs []uint64, minSeverity string) string {
	conditions := []string{"EventRecordID>" + strconv.FormatUint(lastRecord, 10)}
	if maxLevel, ok := windowsMaxLevelForMinSeverity(minSeverity); ok {
		conditions = append(conditions, "Level<="+strconv.Itoa(maxLevel))
	}
	if len(eventIDs) > 0 {
		parts := make([]string, 0, len(eventIDs))
		for _, eventID := range eventIDs {
			parts = append(parts, "EventID="+strconv.FormatUint(eventID, 10))
		}
		conditions = append(conditions, "("+strings.Join(parts, " or ")+")")
	}
	if len(providers) > 0 {
		parts := make([]string, 0, len(providers))
		for _, provider := range providers {
			parts = append(parts, "Provider[@Name='"+provider+"']")
		}
		conditions = append(conditions, "("+strings.Join(parts, " or ")+")")
	}
	return "*[System[" + strings.Join(conditions, " and ") + "]]"
}

func windowsMaxLevelForMinSeverity(minSeverity string) (int, bool) {
	switch normalizeSeverityString(minSeverity) {
	case "critical", "alert", "emergency":
		return 1, true
	case "error":
		return 2, true
	case "warning":
		return 3, true
	case "info", "notice":
		return 4, true
	case "debug", "trace", "verbose":
		return 5, true
	default:
		return 0, false
	}
}

func windowsEventMatches(ev logEvent, providers []string, eventIDs []uint64) bool {
	if len(eventIDs) > 0 && !uint64InList(ev.EventID, eventIDs) {
		return false
	}
	if len(providers) > 0 && !stringInListFold(ev.Provider, providers) {
		return false
	}
	return true
}

func sanitizeWindowsEventIDs(values []string) []uint64 {
	out := make([]uint64, 0, len(values))
	for _, value := range values {
		id, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
		if err == nil && id > 0 {
			out = append(out, id)
		}
	}
	return out
}

func sanitizeWindowsProviders(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		clean := strings.TrimSpace(value)
		if clean != "" && isSafeWindowsProvider(clean) {
			out = append(out, clean)
		}
	}
	return out
}

func isSafeWindowsProvider(value string) bool {
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		if strings.ContainsRune(" ._@:/-{}", r) {
			continue
		}
		return false
	}
	return true
}

func uint64InList(value uint64, values []uint64) bool {
	for _, current := range values {
		if value == current {
			return true
		}
	}
	return false
}

func stringInListFold(value string, values []string) bool {
	for _, current := range values {
		if strings.EqualFold(value, current) {
			return true
		}
	}
	return false
}

func windowsCategory(eventID uint64, msg string) string {
	switch eventID {
	case 4625, 4771, 4768, 4769:
		return "auth_fail"
	case 4624:
		return "auth_success"
	case 4672:
		return "privilege_assigned"
	case 4688:
		return "process_start"
	case 4697, 7045:
		return "service_install"
	case 4720:
		return "user_create"
	case 4726:
		return "user_delete"
	case 4728, 4732:
		return "group_add"
	case 4735:
		return "group_change"
	case 4740:
		return "account_locked"
	default:
		l := strings.ToLower(msg)
		if strings.Contains(l, "failed logon") {
			return "auth_fail"
		}
		return ""
	}
}

func loadCursorWin(path string) map[string]uint64 {
	b, err := os.ReadFile(path)
	if err != nil {
		return map[string]uint64{}
	}
	var cur map[string]uint64
	if err := json.Unmarshal(b, &cur); err != nil {
		return map[string]uint64{}
	}
	return cur
}

func saveCursorWin(path string, cur map[string]uint64) error {
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
