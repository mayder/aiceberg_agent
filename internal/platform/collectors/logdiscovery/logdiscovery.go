package logdiscovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	psnet "github.com/shirou/gopsutil/v3/net"
	ps "github.com/shirou/gopsutil/v3/process"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

const (
	schemaVersion        = "log_source_discovery_v1"
	collectorName        = "log_source_discovery"
	collectorVersion     = "1-readonly"
	defaultMaxCandidates = 200
)

type Collector struct {
	cfg        config.Config
	prefs      func() config.CollectPrefs
	knownPaths []knownPath
	now        func() time.Time
	hostname   func() (string, error)
}

type knownPath struct {
	Path                string
	Kind                string
	Product             string
	ServiceName         string
	RecommendedCategory string
	SOCSourceType       string
	SOCEligible         string
	Permissions         []string
}

type payload struct {
	SchemaVersion    string         `json:"schema_version"`
	CollectorVersion string         `json:"collector_version"`
	AgentID          int            `json:"agent_id"`
	AssetID          any            `json:"asset_id"`
	Host             string         `json:"host"`
	OS               string         `json:"os"`
	CollectedAt      string         `json:"collected_at"`
	ScanPolicy       map[string]any `json:"scan_policy"`
	Capabilities     map[string]any `json:"capabilities"`
	Candidates       []Candidate    `json:"candidates"`
	Gaps             []gap          `json:"gaps"`
	RedactionSummary map[string]any `json:"redaction_summary"`
}

type Candidate struct {
	Fingerprint         string   `json:"fingerprint"`
	Kind                string   `json:"kind"`
	Product             string   `json:"product"`
	ServiceName         string   `json:"service_name"`
	ProcessName         string   `json:"process_name"`
	Port                int      `json:"port"`
	Listener            string   `json:"listener"`
	Path                string   `json:"path"`
	Channel             string   `json:"channel"`
	Unit                string   `json:"unit"`
	Container           string   `json:"container"`
	Pod                 string   `json:"pod"`
	Namespace           string   `json:"namespace"`
	Runtime             string   `json:"runtime"`
	Version             string   `json:"version"`
	Confidence          float64  `json:"confidence"`
	Evidence            []string `json:"evidence"`
	RecommendedCategory string   `json:"recommended_category"`
	UsefulFor           []string `json:"useful_for"`
	SOCSourceType       string   `json:"soc_source_type"`
	SOCEligible         string   `json:"soc_eligible"`
	OriginConfidence    string   `json:"origin_confidence"`
	MinSeverity         string   `json:"min_severity"`
	EstimatedVolume     string   `json:"estimated_volume"`
	UsefulnessScore     float64  `json:"usefulness_score"`
	RiskScore           float64  `json:"risk_score"`
	PermissionsRequired []string `json:"permissions_required"`
	RedactionPolicy     string   `json:"redaction_policy"`
	RetentionHint       string   `json:"retention_hint"`
	Freshness           string   `json:"freshness"`
	Status              string   `json:"status"`
	RollbackRef         string   `json:"rollback_ref"`
}

type gap struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Scope   string `json:"scope"`
}

func New(cfg config.Config, prefsProvider func() config.CollectPrefs) ports.Collector {
	return newCollector(cfg, prefsProvider, defaultKnownPaths())
}

func newCollector(cfg config.Config, prefsProvider func() config.CollectPrefs, paths []knownPath) *Collector {
	if prefsProvider == nil {
		prefsProvider = func() config.CollectPrefs { return config.CollectPrefs{} }
	}
	return &Collector{
		cfg:        cfg,
		prefs:      prefsProvider,
		knownPaths: paths,
		now:        time.Now,
		hostname:   os.Hostname,
	}
}

func (c *Collector) Name() string { return collectorName }

func (c *Collector) Interval() time.Duration { return c.effectiveInterval(c.prefs()) }

func (c *Collector) Collect(ctx context.Context) ([]byte, error) {
	p := c.prefs()
	if !c.effectiveEnabled(p) {
		return nil, nil
	}
	host, _ := c.hostname()
	gaps := make([]gap, 0, 4)
	candidates := make([]Candidate, 0, c.maxCandidates(p))
	candidates = append(candidates, c.discoverStructuredChannels()...)
	candidates = append(candidates, c.discoverKnownLogFiles(&gaps)...)
	candidates = append(candidates, c.discoverSystemdUnits(&gaps)...)
	candidates = append(candidates, c.discoverRuntimes()...)
	candidates = append(candidates, c.discoverProcesses(ctx, &gaps)...)
	candidates = append(candidates, c.discoverListeners(ctx, &gaps)...)
	candidates = c.deduplicate(candidates, c.maxCandidates(p))

	body := payload{
		SchemaVersion:    schemaVersion,
		CollectorVersion: collectorVersion,
		AgentID:          c.cfg.AgentID,
		AssetID:          nil,
		Host:             host,
		OS:               runtime.GOOS,
		CollectedAt:      c.now().UTC().Format(time.RFC3339),
		ScanPolicy: map[string]any{
			"bounded":              true,
			"read_only":            true,
			"max_candidates":       c.maxCandidates(p),
			"max_evidence_bytes":   c.maxEvidenceBytes(p),
			"activates_collection": false,
			"default_min_severity": "error",
		},
		Capabilities:     c.capabilities(p),
		Candidates:       candidates,
		Gaps:             uniqueGaps(gaps),
		RedactionSummary: map[string]any{"secret_values_removed": true, "raw_command_line": false},
	}
	raw, err := json.Marshal(map[string]any{schemaVersion: body})
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (c *Collector) effectiveEnabled(p config.CollectPrefs) bool {
	if strings.TrimSpace(p.Version) != "" {
		return p.LogDiscoveryEnabled
	}
	return c.cfg.LogDiscoveryEnabled
}

func (c *Collector) effectiveInterval(p config.CollectPrefs) time.Duration {
	if p.LogDiscoveryIntervalSec > 0 {
		return time.Duration(p.LogDiscoveryIntervalSec) * time.Second
	}
	if c.cfg.LogDiscoveryInterval > 0 {
		return c.cfg.LogDiscoveryInterval
	}
	return 5 * time.Minute
}

func (c *Collector) maxCandidates(p config.CollectPrefs) int {
	max := c.cfg.LogDiscoveryMaxCandidates
	if p.LogDiscoveryMaxCandidates > 0 {
		max = p.LogDiscoveryMaxCandidates
	}
	if max <= 0 {
		max = defaultMaxCandidates
	}
	if max > 1000 {
		return 1000
	}
	return max
}

func (c *Collector) maxEvidenceBytes(p config.CollectPrefs) int {
	max := c.cfg.LogDiscoveryMaxEvidenceBytes
	if p.LogDiscoveryMaxEvidenceBytes > 0 {
		max = p.LogDiscoveryMaxEvidenceBytes
	}
	if max <= 0 {
		return 2048
	}
	return max
}

func (c *Collector) discoverStructuredChannels() []Candidate {
	if runtime.GOOS != "windows" {
		return nil
	}
	rows := []Candidate{}
	for _, channel := range []string{"Security", "System", "Application", "Microsoft-Windows-Sysmon/Operational"} {
		row := baseCandidate("eventlog_channel", "windows_eventlog")
		row.Channel = channel
		row.ServiceName = "windows_eventlog"
		row.Confidence = 0.92
		row.Evidence = []string{"windows_eventlog_channel:" + channel}
		assignCategory(&row, "soc")
		row.SOCSourceType = "windows_security"
		row.SOCEligible = "yes"
		if channel == "System" || channel == "Application" {
			assignCategory(&row, "observability")
			row.SOCEligible = "conditional"
			row.SOCSourceType = "none"
		}
		row.Fingerprint = fingerprint(row)
		rows = append(rows, row)
	}
	return rows
}

func (c *Collector) discoverKnownLogFiles(gaps *[]gap) []Candidate {
	rows := make([]Candidate, 0, len(c.knownPaths))
	for _, known := range c.knownPaths {
		matches, err := existingPathMatches(known.Path)
		if err != nil {
			*gaps = append(*gaps, gap{Code: "path_permission_denied", Message: "sem permissao para inspecionar fonte local", Scope: known.Path})
			continue
		}
		for _, match := range matches {
			info, statErr := os.Stat(match)
			if statErr != nil || info.IsDir() {
				continue
			}
			row := c.knownPathCandidate(known, match, info)
			rows = append(rows, row)
		}
	}
	return rows
}

func (c *Collector) knownPathCandidate(known knownPath, path string, info os.FileInfo) Candidate {
	row := baseCandidate(known.Kind, known.Product)
	row.ServiceName = known.ServiceName
	row.Path = filepath.Clean(path)
	row.Confidence = 0.88
	row.Evidence = []string{"file_exists", "size:" + strconv.FormatInt(info.Size(), 10)}
	assignCategory(&row, known.RecommendedCategory)
	row.SOCSourceType = known.SOCSourceType
	row.SOCEligible = known.SOCEligible
	row.EstimatedVolume = estimateVolume(info.Size())
	row.Freshness = freshness(c.now(), info.ModTime())
	row.PermissionsRequired = known.Permissions
	row.Fingerprint = fingerprint(row)
	return row
}

func (c *Collector) discoverRuntimes() []Candidate {
	rows := []Candidate{}
	for _, item := range []struct {
		path    string
		product string
		kind    string
		runtime string
	}{
		{c.cfg.ContainerDockerSocket, "docker", "container_runtime", "docker"},
		{c.cfg.ContainerContainerdSocket, "containerd", "container_runtime", "containerd"},
		{c.cfg.KubernetesTokenPath, "kubernetes", "kubernetes", "kubernetes"},
	} {
		if strings.TrimSpace(item.path) == "" || !pathExists(item.path) {
			continue
		}
		row := baseCandidate(item.kind, item.product)
		row.Runtime = item.runtime
		row.Path = filepath.Clean(item.path)
		row.Confidence = 0.9
		row.Evidence = []string{"path_exists:" + row.Path}
		assignCategory(&row, "observability")
		row.SOCSourceType = "cloud"
		row.SOCEligible = "conditional"
		row.PermissionsRequired = []string{"read:" + row.Path}
		row.Fingerprint = fingerprint(row)
		rows = append(rows, row)
	}
	if c.cfg.OTLPEnabled || strings.TrimSpace(c.cfg.OTLPHTTPAddr) != "" {
		row := baseCandidate("telemetry_endpoint", "opentelemetry")
		row.Listener = strings.TrimSpace(c.cfg.OTLPHTTPAddr)
		row.Port = parsePort(row.Listener)
		row.Confidence = 0.8
		row.Evidence = []string{"otlp_configured"}
		assignCategory(&row, "observability")
		row.SOCEligible = "conditional"
		row.SOCSourceType = "none"
		row.Fingerprint = fingerprint(row)
		rows = append(rows, row)
	}
	if runtime.GOOS != "windows" && pathExists("/run/systemd/journal/socket") {
		row := baseCandidate("journal", "journald")
		row.Runtime = "systemd"
		row.Path = "/run/systemd/journal/socket"
		row.Confidence = 0.88
		row.Evidence = []string{"journald_socket"}
		assignCategory(&row, "observability")
		row.SOCEligible = "conditional"
		row.Fingerprint = fingerprint(row)
		rows = append(rows, row)
	}
	if namespace := kubernetesNamespace(); namespace != "" {
		row := baseCandidate("kubernetes_context", "kubernetes")
		row.Runtime = "kubernetes"
		row.Namespace = namespace
		row.Pod = hostEnv("HOSTNAME")
		row.Confidence = 0.82
		row.Evidence = []string{"serviceaccount_namespace", "pod_hostname:" + sanitizeText(row.Pod, 80)}
		assignCategory(&row, "observability")
		row.SOCSourceType = "cloud"
		row.SOCEligible = "conditional"
		row.Fingerprint = fingerprint(row)
		rows = append(rows, row)
	}
	return rows
}

func (c *Collector) discoverSystemdUnits(gaps *[]gap) []Candidate {
	if runtime.GOOS == "windows" {
		return nil
	}
	dirs := []string{"/etc/systemd/system", "/run/systemd/system", "/lib/systemd/system", "/usr/lib/systemd/system"}
	rows := []Candidate{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if isPermission(err) {
				*gaps = append(*gaps, gap{Code: "systemd_permission_denied", Message: "sem permissao para listar units", Scope: dir})
			}
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".service") {
				continue
			}
			product, category := classifyProcess(strings.TrimSuffix(entry.Name(), ".service"), "")
			if product == "" {
				continue
			}
			row := baseCandidate("systemd_unit", product)
			row.ServiceName = product
			row.Unit = entry.Name()
			row.Path = filepath.Join(dir, entry.Name())
			row.Runtime = "systemd"
			row.Confidence = 0.74
			row.Evidence = []string{"systemd_unit:" + entry.Name()}
			assignCategory(&row, category)
			row.SOCSourceType = socTypeForProduct(product)
			row.SOCEligible = socEligibleForCategory(category)
			row.Fingerprint = fingerprint(row)
			rows = append(rows, row)
			if len(rows) >= c.maxCandidates(c.prefs()) {
				return rows
			}
		}
	}
	return rows
}

func (c *Collector) discoverProcesses(ctx context.Context, gaps *[]gap) []Candidate {
	procs, err := ps.ProcessesWithContext(ctx)
	if err != nil {
		*gaps = append(*gaps, gap{Code: "process_scan_failed", Message: err.Error(), Scope: "processes"})
		return nil
	}
	rows := []Candidate{}
	for _, proc := range procs {
		name, _ := proc.NameWithContext(ctx)
		cmdline, _ := proc.CmdlineWithContext(ctx)
		exe, _ := proc.ExeWithContext(ctx)
		product, category := classifyProcess(name, cmdline)
		if product == "" {
			continue
		}
		row := baseCandidate("process", product)
		row.ProcessName = safeToken(name, 80)
		row.ServiceName = product
		row.Path = safePath(exe)
		row.Confidence = 0.68
		row.Evidence = []string{"process:" + row.ProcessName, "cmdline:" + sanitizeText(cmdline, 160)}
		assignCategory(&row, category)
		row.SOCSourceType = socTypeForProduct(product)
		row.SOCEligible = socEligibleForCategory(category)
		row.Fingerprint = fingerprint(row)
		rows = append(rows, row)
		if len(rows) >= c.maxCandidates(c.prefs()) {
			break
		}
	}
	return rows
}

func (c *Collector) discoverListeners(ctx context.Context, gaps *[]gap) []Candidate {
	conns, err := psnet.ConnectionsWithContext(ctx, "tcp")
	if err != nil {
		*gaps = append(*gaps, gap{Code: "listener_scan_failed", Message: err.Error(), Scope: "listeners"})
		return nil
	}
	rows := []Candidate{}
	for _, conn := range conns {
		if !strings.EqualFold(conn.Status, "LISTEN") || conn.Laddr.Port == 0 {
			continue
		}
		product, category := classifyPort(int(conn.Laddr.Port))
		if product == "" {
			continue
		}
		row := baseCandidate("listener", product)
		row.Port = int(conn.Laddr.Port)
		row.Listener = fmt.Sprintf("%s:%d", conn.Laddr.IP, conn.Laddr.Port)
		row.Confidence = 0.7
		row.Evidence = []string{"tcp_listen:" + strconv.Itoa(row.Port)}
		assignCategory(&row, category)
		row.SOCSourceType = socTypeForProduct(product)
		row.SOCEligible = socEligibleForCategory(category)
		row.Fingerprint = fingerprint(row)
		rows = append(rows, row)
	}
	return rows
}

func (c *Collector) capabilities(p config.CollectPrefs) map[string]any {
	return map[string]any{
		"windows_eventlog":  runtime.GOOS == "windows",
		"journald_socket":   pathExists("/run/systemd/journal/socket"),
		"docker_socket":     pathExists(c.cfg.ContainerDockerSocket),
		"containerd_socket": pathExists(c.cfg.ContainerContainerdSocket),
		"kubernetes_token":  pathExists(c.cfg.KubernetesTokenPath),
		"otlp_enabled":      c.cfg.OTLPEnabled || p.OTLPEnabled,
		"min_severity":      "error",
	}
}

func (c *Collector) deduplicate(rows []Candidate, limit int) []Candidate {
	seen := map[string]bool{}
	out := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		if row.Fingerprint == "" {
			row.Fingerprint = fingerprint(row)
		}
		if seen[row.Fingerprint] {
			continue
		}
		seen[row.Fingerprint] = true
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].UsefulnessScore == out[j].UsefulnessScore {
			return out[i].Fingerprint < out[j].Fingerprint
		}
		return out[i].UsefulnessScore > out[j].UsefulnessScore
	})
	if limit > 0 && len(out) > limit {
		return out[:limit]
	}
	return out
}

func baseCandidate(kind, product string) Candidate {
	category := "observability"
	return Candidate{
		Kind:                kind,
		Product:             product,
		RecommendedCategory: category,
		UsefulFor:           usefulFor(category),
		SOCSourceType:       "none",
		SOCEligible:         "conditional",
		OriginConfidence:    "inferred",
		MinSeverity:         "error",
		EstimatedVolume:     "unknown",
		UsefulnessScore:     usefulness(kind, product),
		RiskScore:           risk(product),
		PermissionsRequired: []string{},
		RedactionPolicy:     "secret_pattern_redaction",
		RetentionHint:       "follow_remote_log_retention",
		Freshness:           "unknown",
		Status:              "discovered",
		RollbackRef:         "disable_log_source_discovery_or_reject_source",
	}
}

func assignCategory(row *Candidate, category string) {
	row.RecommendedCategory = category
	row.UsefulFor = usefulFor(category)
}

func usefulFor(category string) []string {
	switch category {
	case "soc":
		return []string{"log", "soc", "troubleshooting"}
	case "noc":
		return []string{"log", "noc", "troubleshooting"}
	default:
		return []string{"log", "noc", "troubleshooting"}
	}
}

func defaultKnownPaths() []knownPath {
	if runtime.GOOS == "windows" {
		return []knownPath{
			{Path: `C:\inetpub\logs\LogFiles\W3SVC1\u_ex*.log`, Kind: "log_glob", Product: "iis", ServiceName: "iis", RecommendedCategory: "observability", SOCSourceType: "waf", SOCEligible: "conditional", Permissions: []string{"read:iis_logs"}},
			{Path: `C:\ProgramData\Microsoft\Windows Defender\Support\MPLog*.log`, Kind: "log_glob", Product: "windows_defender", ServiceName: "defender", RecommendedCategory: "soc", SOCSourceType: "edr", SOCEligible: "yes", Permissions: []string{"read:defender_logs"}},
		}
	}
	return []knownPath{
		{Path: "/var/log/nginx/error.log", Kind: "log_file", Product: "nginx", ServiceName: "nginx", RecommendedCategory: "observability", SOCSourceType: "none", SOCEligible: "conditional", Permissions: []string{"read:/var/log/nginx"}},
		{Path: "/var/log/nginx/*.log", Kind: "log_glob", Product: "nginx", ServiceName: "nginx", RecommendedCategory: "observability", SOCSourceType: "none", SOCEligible: "conditional", Permissions: []string{"read:/var/log/nginx"}},
		{Path: "/var/log/apache2/error.log", Kind: "log_file", Product: "apache", ServiceName: "apache", RecommendedCategory: "observability", SOCSourceType: "none", SOCEligible: "conditional", Permissions: []string{"read:/var/log/apache2"}},
		{Path: "/var/log/apache2/*.log", Kind: "log_glob", Product: "apache", ServiceName: "apache", RecommendedCategory: "observability", SOCSourceType: "none", SOCEligible: "conditional", Permissions: []string{"read:/var/log/apache2"}},
		{Path: "/var/log/httpd/error_log", Kind: "log_file", Product: "apache", ServiceName: "httpd", RecommendedCategory: "observability", SOCSourceType: "none", SOCEligible: "conditional", Permissions: []string{"read:/var/log/httpd"}},
		{Path: "/var/log/httpd/*log", Kind: "log_glob", Product: "apache", ServiceName: "httpd", RecommendedCategory: "observability", SOCSourceType: "none", SOCEligible: "conditional", Permissions: []string{"read:/var/log/httpd"}},
		{Path: "/var/log/auth.log", Kind: "log_file", Product: "linux_auth", ServiceName: "auth", RecommendedCategory: "soc", SOCSourceType: "linux_security", SOCEligible: "yes", Permissions: []string{"read:/var/log/auth.log"}},
		{Path: "/var/log/secure", Kind: "log_file", Product: "linux_auth", ServiceName: "auth", RecommendedCategory: "soc", SOCSourceType: "linux_security", SOCEligible: "yes", Permissions: []string{"read:/var/log/secure"}},
		{Path: "/var/log/syslog", Kind: "log_file", Product: "linux_syslog", ServiceName: "syslog", RecommendedCategory: "observability", SOCSourceType: "none", SOCEligible: "conditional", Permissions: []string{"read:/var/log/syslog"}},
		{Path: "/var/log/messages", Kind: "log_file", Product: "linux_syslog", ServiceName: "syslog", RecommendedCategory: "observability", SOCSourceType: "none", SOCEligible: "conditional", Permissions: []string{"read:/var/log/messages"}},
		{Path: "/var/log/plesk/panel.log", Kind: "log_file", Product: "plesk", ServiceName: "plesk", RecommendedCategory: "observability", SOCSourceType: "none", SOCEligible: "conditional", Permissions: []string{"read:/var/log/plesk"}},
		{Path: "/var/log/mysql/error.log", Kind: "log_file", Product: "mysql", ServiceName: "mysql", RecommendedCategory: "observability", SOCSourceType: "none", SOCEligible: "conditional", Permissions: []string{"read:/var/log/mysql"}},
		{Path: "/var/log/mysql/*.log", Kind: "log_glob", Product: "mysql", ServiceName: "mysql", RecommendedCategory: "observability", SOCSourceType: "none", SOCEligible: "conditional", Permissions: []string{"read:/var/log/mysql"}},
		{Path: "/var/log/mariadb/*.log", Kind: "log_glob", Product: "mysql", ServiceName: "mariadb", RecommendedCategory: "observability", SOCSourceType: "none", SOCEligible: "conditional", Permissions: []string{"read:/var/log/mariadb"}},
		{Path: "/var/log/postgresql/*.log", Kind: "log_glob", Product: "postgresql", ServiceName: "postgresql", RecommendedCategory: "observability", SOCSourceType: "none", SOCEligible: "conditional", Permissions: []string{"read:/var/log/postgresql"}},
		{Path: "/var/log/redis/*.log", Kind: "log_glob", Product: "redis", ServiceName: "redis", RecommendedCategory: "observability", SOCSourceType: "none", SOCEligible: "conditional", Permissions: []string{"read:/var/log/redis"}},
		{Path: "/var/log/rabbitmq/*.log", Kind: "log_glob", Product: "rabbitmq", ServiceName: "rabbitmq", RecommendedCategory: "observability", SOCSourceType: "none", SOCEligible: "conditional", Permissions: []string{"read:/var/log/rabbitmq"}},
		{Path: "/var/log/mongodb/*.log", Kind: "log_glob", Product: "mongodb", ServiceName: "mongodb", RecommendedCategory: "observability", SOCSourceType: "none", SOCEligible: "conditional", Permissions: []string{"read:/var/log/mongodb"}},
		{Path: "/var/log/php*-fpm.log", Kind: "log_glob", Product: "php_fpm", ServiceName: "php-fpm", RecommendedCategory: "observability", SOCSourceType: "none", SOCEligible: "conditional", Permissions: []string{"read:/var/log"}},
		{Path: "/var/log/tomcat*/catalina.out", Kind: "log_glob", Product: "tomcat", ServiceName: "tomcat", RecommendedCategory: "observability", SOCSourceType: "none", SOCEligible: "conditional", Permissions: []string{"read:/var/log/tomcat"}},
		{Path: "/var/lib/docker/containers/*/*-json.log", Kind: "container_log_glob", Product: "docker", ServiceName: "docker", RecommendedCategory: "observability", SOCSourceType: "none", SOCEligible: "conditional", Permissions: []string{"read:/var/lib/docker/containers"}},
	}
}

func fingerprint(row Candidate) string {
	parts := []string{
		schemaVersion,
		row.Kind,
		row.Product,
		row.ServiceName,
		row.ProcessName,
		strconv.Itoa(row.Port),
		row.Path,
		row.Channel,
		row.Unit,
		row.Runtime,
	}
	sum := sha256.Sum256([]byte(strings.ToLower(strings.Join(parts, "|"))))
	return hex.EncodeToString(sum[:])
}

func classifyProcess(name, cmdline string) (string, string) {
	text := strings.ToLower(name + " " + cmdline)
	rules := []struct{ match, product, category string }{
		{"nginx", "nginx", "observability"},
		{"apache", "apache", "observability"},
		{"httpd", "apache", "observability"},
		{"php-fpm", "php_fpm", "observability"},
		{"tomcat", "tomcat", "observability"},
		{"w3wp", "iis", "observability"},
		{"sshd", "openssh", "soc"},
		{"mysqld", "mysql", "observability"},
		{"postgres", "postgresql", "observability"},
		{"sqlservr", "sql_server", "observability"},
		{"redis-server", "redis", "observability"},
		{"rabbitmq", "rabbitmq", "observability"},
		{"mongod", "mongodb", "observability"},
		{"java", "java_app", "observability"},
		{"dotnet", "dotnet_app", "observability"},
		{"node", "node_app", "observability"},
		{"python", "python_app", "observability"},
	}
	for _, rule := range rules {
		if strings.Contains(text, rule.match) {
			return rule.product, rule.category
		}
	}
	return "", ""
}

func classifyPort(port int) (string, string) {
	switch port {
	case 22:
		return "openssh", "soc"
	case 80, 443, 8080, 8443:
		return "web_server", "observability"
	case 1433:
		return "sql_server", "observability"
	case 3306:
		return "mysql", "observability"
	case 5432:
		return "postgresql", "observability"
	case 6379:
		return "redis", "observability"
	case 5672, 15672:
		return "rabbitmq", "observability"
	case 9200:
		return "elasticsearch", "observability"
	case 27017:
		return "mongodb", "observability"
	case 4317, 4318:
		return "opentelemetry", "observability"
	case 8125, 8126:
		return "statsd_apm", "observability"
	default:
		return "", ""
	}
}

func socTypeForProduct(product string) string {
	switch product {
	case "openssh", "linux_auth":
		return "linux_security"
	case "windows_defender":
		return "edr"
	default:
		return "none"
	}
}

func socEligibleForCategory(category string) string {
	if category == "soc" {
		return "yes"
	}
	return "conditional"
}

func usefulness(kind, product string) float64 {
	score := 0.55
	if kind == "eventlog_channel" || kind == "log_file" {
		score += 0.25
	}
	if product == "linux_auth" || product == "windows_eventlog" || product == "nginx" || product == "apache" || product == "iis" {
		score += 0.15
	}
	if score > 0.99 {
		return 0.99
	}
	return score
}

func risk(product string) float64 {
	switch product {
	case "linux_auth", "windows_eventlog", "windows_defender", "openssh":
		return 0.85
	case "mysql", "postgresql", "sql_server", "redis", "rabbitmq", "mongodb":
		return 0.7
	default:
		return 0.45
	}
}

func estimateVolume(size int64) string {
	switch {
	case size > 100*1024*1024:
		return "high"
	case size > 5*1024*1024:
		return "medium"
	default:
		return "low"
	}
}

func freshness(now time.Time, mod time.Time) string {
	if mod.IsZero() {
		return "unknown"
	}
	age := now.Sub(mod)
	switch {
	case age < 24*time.Hour:
		return "fresh"
	case age < 30*24*time.Hour:
		return "recent"
	default:
		return "stale"
	}
}

func parsePort(addr string) int {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		port, _ := strconv.Atoi(addr[i+1:])
		return port
	}
	return 0
}

func pathExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func hostEnv(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func kubernetesNamespace() string {
	if hostEnv("KUBERNETES_SERVICE_HOST") == "" && !pathExists("/var/run/secrets/kubernetes.io/serviceaccount/namespace") {
		return ""
	}
	raw, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err == nil {
		if namespace := strings.TrimSpace(string(raw)); namespace != "" {
			return namespace
		}
	}
	return hostEnv("POD_NAMESPACE")
}

func existingPathMatches(path string) ([]string, error) {
	if strings.ContainsAny(path, "*?[") {
		matches, err := filepath.Glob(path)
		if err != nil {
			return nil, nil
		}
		return matches, nil
	}
	_, err := os.Stat(path)
	if err != nil {
		if isPermission(err) {
			return nil, err
		}
		return nil, nil
	}
	return []string{path}, nil
}

func isPermission(err error) bool {
	return err != nil && (os.IsPermission(err) || errors.Is(err, os.ErrPermission))
}

func safeToken(value string, max int) string {
	value = strings.TrimSpace(value)
	if max > 0 && len(value) > max {
		return value[:max]
	}
	return value
}

func safePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return filepath.Clean(value)
}

func sanitizeText(value string, max int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	fields := strings.Fields(value)
	redactNext := false
	for i, field := range fields {
		lower := strings.ToLower(field)
		if redactNext {
			fields[i] = "[redacted]"
			redactNext = false
			continue
		}
		if strings.Contains(lower, "password") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "authorization") {
			fields[i] = "[redacted]"
			if !strings.Contains(field, "=") && !strings.Contains(field, ":") {
				redactNext = true
			}
		}
	}
	out := strings.Join(fields, " ")
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func uniqueGaps(items []gap) []gap {
	seen := map[string]bool{}
	out := []gap{}
	for _, item := range items {
		key := item.Code + "|" + item.Scope
		if item.Code == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}
