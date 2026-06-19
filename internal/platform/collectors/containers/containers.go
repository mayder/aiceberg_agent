package containers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/soclog"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

const schemaVersion = 1

type collector struct {
	prefs          func() config.CollectPrefs
	baseEnabled    bool
	runtime        string
	socket         string
	containerdSock string
	containerdNS   string
	ctrPath        string
	interval       time.Duration
	maxItems       int
	include        string
	exclude        string
	logsEnabled    bool
	logCursorPath  string
	logMaxLines    int
	logMaxBytes    int
}

func New(cfg config.Config, prefsProvider func() config.CollectPrefs) ports.Collector {
	interval := cfg.ContainerInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	maxItems := cfg.ContainerMaxItems
	if maxItems <= 0 {
		maxItems = 200
	}
	return &collector{
		prefs:          prefsProvider,
		baseEnabled:    cfg.ContainerEnabled,
		runtime:        cfg.ContainerRuntime,
		socket:         cfg.ContainerDockerSocket,
		containerdSock: cfg.ContainerContainerdSocket,
		containerdNS:   cfg.ContainerContainerdNamespace,
		ctrPath:        cfg.ContainerCtrPath,
		interval:       interval,
		maxItems:       maxItems,
		include:        cfg.ContainerIncludeRegex,
		exclude:        cfg.ContainerExcludeRegex,
		logsEnabled:    cfg.ContainerLogsEnabled,
		logCursorPath:  cfg.ContainerLogsCursorPath,
		logMaxLines:    cfg.ContainerLogsMaxLines,
		logMaxBytes:    cfg.ContainerLogsMaxBytes,
	}
}

func (c *collector) Name() string { return "containers" }

func (c *collector) Interval() time.Duration { return c.interval }

func (c *collector) Collect(ctx context.Context) ([]byte, error) {
	if !c.enabled() {
		return nil, nil
	}
	runtimeName, dockerSocket, containerdSocket, containerdNS, ctrPath := c.effectiveRuntimeConfig()
	if runtimeName == "containerd" {
		return c.collectContainerd(ctx, ctrPath, containerdSocket, containerdNS)
	}
	payload, err := c.collectDocker(ctx, dockerSocket)
	if err == nil || runtimeName == "docker" {
		return payload, err
	}
	if hasUnixSocket(containerdSocket) {
		if fallback, fallbackErr := c.collectContainerd(ctx, ctrPath, containerdSocket, containerdNS); fallbackErr == nil {
			return fallback, nil
		}
	}
	return nil, err
}

func (c *collector) collectDocker(ctx context.Context, socket string) ([]byte, error) {
	if strings.TrimSpace(socket) == "" {
		return nil, errors.New("docker socket vazio")
	}
	client := dockerClient(socket)
	containers, err := listDockerContainers(ctx, client, c.maxItems)
	if err != nil {
		return nil, err
	}
	inspectByID := map[string]dockerInspect{}
	for _, row := range containers {
		id := shortID(row.ID)
		if id == "" {
			continue
		}
		if inspect, err := fetchDockerInspect(ctx, client, id); err == nil {
			inspectByID[id] = inspect
		}
	}
	include, exclude := c.effectiveFilters()
	containers = filterContainers(containers, inspectByID, include, exclude)
	statsByID := map[string]dockerStats{}
	for _, row := range containers {
		id := shortID(row.ID)
		if id == "" {
			continue
		}
		stats, err := fetchDockerStats(ctx, client, id)
		if err == nil {
			statsByID[id] = stats
		}
	}
	payload := map[string]any{
		"containers": map[string]any{
			"schema_version":       schemaVersion,
			"source":               "docker_socket",
			"items":                normalizeContainers(containers, statsByID, inspectByID),
			"logs":                 c.collectContainerLogs(containers, inspectByID),
			"autodiscovery_checks": autodiscoveryChecks(containers),
			"dropped_count":        maxInt(0, len(containers)-c.maxItems),
		},
	}
	return json.Marshal(payload)
}

func (c *collector) collectContainerd(ctx context.Context, ctrPath, socket, namespace string) ([]byte, error) {
	rows, err := listContainerdContainers(ctx, ctrPath, socket, namespace, c.maxItems)
	if err != nil {
		return nil, err
	}
	include, exclude := c.effectiveFilters()
	rows = filterContainers(rows, nil, include, exclude)
	payload := map[string]any{
		"containers": map[string]any{
			"schema_version":       schemaVersion,
			"source":               "containerd_ctr",
			"items":                normalizeContainers(rows, nil, nil),
			"logs":                 nil,
			"autodiscovery_checks": autodiscoveryChecks(rows),
			"dropped_count":        maxInt(0, len(rows)-c.maxItems),
		},
	}
	return json.Marshal(payload)
}

func (c *collector) effectiveRuntime() string {
	runtimeName, _, _, _, _ := c.effectiveRuntimeConfig()
	return runtimeName
}

func (c *collector) effectiveRuntimeConfig() (string, string, string, string, string) {
	runtimeName := strings.ToLower(strings.TrimSpace(c.runtime))
	dockerSocket := c.socket
	containerdSocket := c.containerdSock
	containerdNS := c.containerdNS
	ctrPath := c.ctrPath
	if c.prefs != nil {
		prefs := c.prefs()
		if p := strings.ToLower(strings.TrimSpace(prefs.ContainerRuntime)); p != "" {
			runtimeName = p
		}
		if strings.TrimSpace(prefs.ContainerDockerSocket) != "" {
			dockerSocket = prefs.ContainerDockerSocket
		}
		if strings.TrimSpace(prefs.ContainerContainerdSocket) != "" {
			containerdSocket = prefs.ContainerContainerdSocket
		}
		if strings.TrimSpace(prefs.ContainerContainerdNS) != "" {
			containerdNS = prefs.ContainerContainerdNS
		}
		if strings.TrimSpace(prefs.ContainerCtrPath) != "" {
			ctrPath = prefs.ContainerCtrPath
		}
	}
	switch runtimeName {
	case "docker", "containerd":
	default:
		runtimeName = "auto"
	}
	if strings.TrimSpace(containerdSocket) == "" {
		containerdSocket = "/run/containerd/containerd.sock"
	}
	if strings.TrimSpace(containerdNS) == "" {
		containerdNS = "k8s.io"
	}
	if strings.TrimSpace(ctrPath) == "" {
		ctrPath = "ctr"
	}
	return runtimeName, dockerSocket, containerdSocket, containerdNS, ctrPath
}

func (c *collector) effectiveLogSettings() (bool, string, int, int) {
	enabled := c.logsEnabled
	maxLines := c.logMaxLines
	maxBytes := c.logMaxBytes
	if c.prefs != nil {
		p := c.prefs()
		if p.ContainerLogsEnabled {
			enabled = true
		}
		if p.ContainerLogsMaxLines > 0 {
			maxLines = p.ContainerLogsMaxLines
		}
		if p.ContainerLogsMaxBytes > 0 {
			maxBytes = p.ContainerLogsMaxBytes
		}
	}
	if maxLines <= 0 {
		maxLines = 200
	}
	if maxBytes <= 0 {
		maxBytes = 256 * 1024
	}
	return enabled, c.logCursorPath, maxLines, maxBytes
}

func (c *collector) effectiveFilters() (string, string) {
	include, exclude := c.include, c.exclude
	if c.prefs != nil {
		p := c.prefs()
		if strings.TrimSpace(p.ContainerIncludeRegex) != "" {
			include = p.ContainerIncludeRegex
		}
		if strings.TrimSpace(p.ContainerExcludeRegex) != "" {
			exclude = p.ContainerExcludeRegex
		}
	}
	return include, exclude
}

func (c *collector) enabled() bool {
	p := config.CollectPrefs{}
	if c.prefs != nil {
		p = c.prefs()
	}
	if strings.TrimSpace(p.Version) == "" {
		return c.baseEnabled
	}
	return p.ContainerEnabled
}

func dockerClient(socket string) *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socket)
			},
		},
	}
}

func listContainerdContainers(ctx context.Context, ctrPath, socket, namespace string, maxItems int) ([]dockerContainer, error) {
	ctrPath = strings.TrimSpace(ctrPath)
	if ctrPath == "" {
		ctrPath = "ctr"
	}
	socket = strings.TrimSpace(socket)
	if socket == "" {
		socket = "/run/containerd/containerd.sock"
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		namespace = "k8s.io"
	}
	if !hasUnixSocket(socket) {
		return nil, errors.New("containerd socket indisponivel")
	}
	idsRaw, err := runCtr(ctx, ctrPath, socket, namespace, "containers", "list", "--quiet")
	if err != nil {
		return nil, err
	}
	ids := nonEmptyLines(idsRaw)
	limit := maxItems
	if limit <= 0 {
		limit = 200
	}
	if len(ids) > limit {
		ids = ids[:limit]
	}
	out := make([]dockerContainer, 0, len(ids))
	for _, id := range ids {
		infoRaw, err := runCtr(ctx, ctrPath, socket, namespace, "containers", "info", id)
		if err != nil {
			continue
		}
		if row, ok := parseContainerdInfo(infoRaw); ok {
			out = append(out, row)
		}
	}
	return out, nil
}

func runCtr(ctx context.Context, ctrPath, socket, namespace string, args ...string) ([]byte, error) {
	base := []string{"--address", socket, "--namespace", namespace}
	base = append(base, args...)
	cmd := exec.CommandContext(ctx, ctrPath, base...)
	return cmd.Output()
}

func parseContainerdInfo(raw []byte) (dockerContainer, bool) {
	var info struct {
		ID     string            `json:"ID"`
		Image  string            `json:"Image"`
		Labels map[string]string `json:"Labels"`
		Spec   struct {
			Process struct {
				User struct {
					UID uint32 `json:"uid"`
					GID uint32 `json:"gid"`
				} `json:"user"`
			} `json:"process"`
		} `json:"Spec"`
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		return dockerContainer{}, false
	}
	id := strings.TrimSpace(info.ID)
	if id == "" {
		return dockerContainer{}, false
	}
	name := id
	if value := strings.TrimSpace(info.Labels["io.kubernetes.container.name"]); value != "" {
		name = value
	}
	row := dockerContainer{
		ID:     id,
		Names:  []string{"/" + name},
		Image:  info.Image,
		Labels: info.Labels,
		State:  "unknown",
		Status: "containerd",
	}
	return row, true
}

func hasUnixSocket(path string) bool {
	info, err := os.Stat(strings.TrimSpace(path))
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSocket != 0
}

func nonEmptyLines(raw []byte) []string {
	lines := strings.Split(string(raw), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func listDockerContainers(ctx context.Context, client *http.Client, maxItems int) ([]dockerContainer, error) {
	limit := maxItems
	if limit <= 0 {
		limit = 200
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/json?all=1&limit="+url.QueryEscape(intString(limit)), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("docker containers status " + resp.Status)
	}
	var rows []dockerContainer
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, err
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

func fetchDockerStats(ctx context.Context, client *http.Client, id string) (dockerStats, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/"+url.PathEscape(id)+"/stats?stream=false", nil)
	if err != nil {
		return dockerStats{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return dockerStats{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return dockerStats{}, errors.New("docker stats status " + resp.Status)
	}
	var stats dockerStats
	return stats, json.NewDecoder(resp.Body).Decode(&stats)
}

func fetchDockerInspect(ctx context.Context, client *http.Client, id string) (dockerInspect, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/"+url.PathEscape(id)+"/json", nil)
	if err != nil {
		return dockerInspect{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return dockerInspect{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return dockerInspect{}, errors.New("docker inspect status " + resp.Status)
	}
	var inspect dockerInspect
	return inspect, json.NewDecoder(resp.Body).Decode(&inspect)
}

type dockerContainer struct {
	ID         string            `json:"Id"`
	Names      []string          `json:"Names"`
	Image      string            `json:"Image"`
	ImageID    string            `json:"ImageID"`
	Command    string            `json:"Command"`
	Created    int64             `json:"Created"`
	Ports      []map[string]any  `json:"Ports"`
	Labels     map[string]string `json:"Labels"`
	State      string            `json:"State"`
	Status     string            `json:"Status"`
	HostConfig struct {
		NetworkMode string `json:"NetworkMode"`
	} `json:"HostConfig"`
	NetworkSettings struct {
		Networks map[string]any `json:"Networks"`
	} `json:"NetworkSettings"`
}

type dockerStats struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage  uint64   `json:"total_usage"`
			PercpuUsage []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
	} `json:"memory_stats"`
	Networks map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	} `json:"networks"`
	BlkioStats struct {
		IoServiceBytesRecursive []struct {
			Op    string `json:"op"`
			Value uint64 `json:"value"`
		} `json:"io_service_bytes_recursive"`
	} `json:"blkio_stats"`
}

type dockerInspect struct {
	RestartCount int    `json:"RestartCount"`
	LogPath      string `json:"LogPath"`
	Config       struct {
		User string `json:"User"`
	} `json:"Config"`
}

func normalizeContainers(rows []dockerContainer, statsByID map[string]dockerStats, inspectByID map[string]dockerInspect) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		id := shortID(row.ID)
		item := map[string]any{
			"id":           id,
			"name":         firstName(row.Names),
			"image":        row.Image,
			"image_id":     shortID(row.ImageID),
			"state":        row.State,
			"status":       row.Status,
			"created_unix": row.Created,
			"labels":       safeLabels(row.Labels),
			"network_mode": row.HostConfig.NetworkMode,
			"ports":        row.Ports,
			"networks":     networkNames(row.NetworkSettings.Networks),
		}
		if compose := composeService(row.Labels); compose != "" {
			item["compose_service"] = compose
		}
		if namespace := containerNamespace(row.Labels); namespace != "" {
			item["namespace"] = namespace
		}
		if inspect, ok := inspectByID[id]; ok {
			item["restart_count"] = inspect.RestartCount
			if user := strings.TrimSpace(inspect.Config.User); user != "" {
				item["user"] = user
			}
			if logPath := strings.TrimSpace(inspect.LogPath); logPath != "" {
				item["log_path"] = logPath
			}
		}
		if stats, ok := statsByID[id]; ok {
			item["cpu_percent"] = cpuPercent(stats)
			item["memory_usage_bytes"] = stats.MemoryStats.Usage
			item["memory_limit_bytes"] = stats.MemoryStats.Limit
			item["network_rx_bytes"], item["network_tx_bytes"] = networkBytes(stats)
			item["block_read_bytes"], item["block_write_bytes"] = blockBytes(stats)
		}
		out = append(out, item)
	}
	return out
}

func filterContainers(rows []dockerContainer, inspectByID map[string]dockerInspect, includeRegex, excludeRegex string) []dockerContainer {
	include := compileRegex(includeRegex)
	exclude := compileRegex(excludeRegex)
	if include == nil && exclude == nil {
		return rows
	}
	out := make([]dockerContainer, 0, len(rows))
	for _, row := range rows {
		id := shortID(row.ID)
		target := containerFilterText(row, inspectByID[id])
		if include != nil && !include.MatchString(target) {
			continue
		}
		if exclude != nil && exclude.MatchString(target) {
			continue
		}
		out = append(out, row)
	}
	return out
}

func compileRegex(value string) *regexp.Regexp {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	re, err := regexp.Compile(value)
	if err != nil {
		return nil
	}
	return re
}

func containerFilterText(row dockerContainer, inspect dockerInspect) string {
	parts := []string{
		shortID(row.ID),
		firstName(row.Names),
		row.Image,
		row.State,
		row.Status,
		row.HostConfig.NetworkMode,
		composeService(row.Labels),
		containerNamespace(row.Labels),
		"user=" + strings.TrimSpace(inspect.Config.User),
	}
	for key, value := range row.Labels {
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, " ")
}

func safeLabels(labels map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range labels {
		lower := strings.ToLower(k)
		if strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "password") {
			out[k] = "[redacted]"
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func composeService(labels map[string]string) string {
	for _, key := range []string{"com.docker.compose.service", "com.docker.swarm.service.name"} {
		if value := strings.TrimSpace(labels[key]); value != "" {
			return value
		}
	}
	return ""
}

func containerNamespace(labels map[string]string) string {
	for _, key := range []string{"com.docker.compose.project", "io.kubernetes.pod.namespace", "aiceberg.ai/namespace"} {
		if value := strings.TrimSpace(labels[key]); value != "" {
			return value
		}
	}
	return ""
}

func autodiscoveryChecks(rows []dockerContainer) []map[string]any {
	out := []map[string]any{}
	for _, row := range rows {
		id := shortID(row.ID)
		name := firstName(row.Names)
		if raw := strings.TrimSpace(row.Labels["aiceberg.ai/checks"]); raw != "" {
			var checks []map[string]any
			if json.Unmarshal([]byte(raw), &checks) == nil {
				for _, check := range checks {
					addContainerCheckIdentity(check, id, name, row)
					normalizeContainerAutodiscoveryCheck(check, row)
					out = append(out, check)
				}
			}
		}
		for key, value := range row.Labels {
			if !strings.HasPrefix(key, "aiceberg.ai/check.") {
				continue
			}
			check := map[string]any{
				"key":   strings.TrimPrefix(key, "aiceberg.ai/check."),
				"value": value,
			}
			addContainerCheckIdentity(check, id, name, row)
			normalizeContainerAutodiscoveryCheck(check, row)
			out = append(out, check)
		}
	}
	return out
}

func addContainerCheckIdentity(check map[string]any, id, name string, row dockerContainer) {
	check["container_id"] = id
	check["container_name"] = name
	if row.Image != "" {
		check["image"] = row.Image
	}
	if compose := composeService(row.Labels); compose != "" {
		check["service"] = compose
	}
}

func normalizeContainerAutodiscoveryCheck(check map[string]any, row dockerContainer) {
	if _, ok := check["enabled"]; !ok {
		check["enabled"] = true
	}
	kind := normalizeAutodiscoveryKind(fmt.Sprint(firstNonEmpty(check["kind"], check["type"], check["key"])))
	if kind != "" {
		check["kind"] = kind
	}
	target := strings.TrimSpace(fmt.Sprint(firstNonEmpty(check["target"], check["url"], check["value"])))
	if target == "" {
		return
	}
	if kind == "tcp" && !strings.Contains(target, ":") {
		target = "%%host%%:" + target
	}
	if strings.Contains(target, "%%host%%") {
		target = strings.ReplaceAll(target, "%%host%%", firstContainerHost(row))
	}
	check["target"] = target
	if _, ok := check["tags"]; !ok {
		tags := []string{}
		if service := composeService(row.Labels); service != "" {
			tags = append(tags, "service:"+service)
		}
		if namespace := containerNamespace(row.Labels); namespace != "" {
			tags = append(tags, "namespace:"+namespace)
		}
		if len(tags) > 0 {
			check["tags"] = tags
		}
	}
}

func normalizeAutodiscoveryKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "http", "openmetrics", "tcp", "redis", "postgresql", "mysql", "sqlserver", "rabbitmq", "nginx", "apache":
		return strings.ToLower(strings.TrimSpace(kind))
	default:
		return ""
	}
}

func firstContainerHost(row dockerContainer) string {
	for _, name := range row.Names {
		name = strings.Trim(strings.TrimSpace(name), "/")
		if name != "" {
			return name
		}
	}
	return shortID(row.ID)
}

func firstNonEmpty(values ...any) any {
	for _, value := range values {
		if strings.TrimSpace(fmt.Sprint(value)) != "" && fmt.Sprint(value) != "<nil>" {
			return value
		}
	}
	return ""
}

func (c *collector) collectContainerLogs(rows []dockerContainer, inspectByID map[string]dockerInspect) map[string]any {
	enabled, cursorPath, maxLines, maxBytes := c.effectiveLogSettings()
	if !enabled {
		return nil
	}
	cursor := loadLogCursor(cursorPath)
	var events []map[string]any
	dropped := 0
	for _, row := range rows {
		if len(events) >= maxLines {
			break
		}
		id := shortID(row.ID)
		inspect, ok := inspectByID[id]
		if !ok || strings.TrimSpace(inspect.LogPath) == "" {
			continue
		}
		read, drop := readContainerLogFile(row, inspect, cursor, maxLines-len(events), maxBytes)
		events = append(events, read...)
		dropped += drop
	}
	_ = saveLogCursor(cursorPath, cursor)
	if len(events) == 0 && dropped == 0 {
		return nil
	}
	return map[string]any{
		"schema_version": schemaVersion,
		"events":         events,
		"dropped_count":  dropped,
	}
}

func readContainerLogFile(row dockerContainer, inspect dockerInspect, cursor map[string]int64, maxLines, maxBytes int) ([]map[string]any, int) {
	if maxLines <= 0 {
		return nil, 0
	}
	path := strings.TrimSpace(inspect.LogPath)
	f, err := os.Open(path)
	if err != nil {
		return nil, 0
	}
	defer f.Close()
	offset := cursor[path]
	if info, err := f.Stat(); err == nil && offset > info.Size() {
		offset = 0
	}
	if offset > 0 {
		_, _ = f.Seek(offset, 0)
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024), maxBytes)
	var events []map[string]any
	dropped := 0
	for scanner.Scan() {
		if len(events) >= maxLines {
			dropped++
			continue
		}
		line := scanner.Text()
		event, ok := parseContainerLogLine(row, inspect, line, maxBytes)
		if !ok {
			dropped++
			continue
		}
		events = append(events, event)
	}
	if pos, err := f.Seek(0, 1); err == nil {
		cursor[path] = pos
	}
	return events, dropped
}

func parseContainerLogLine(row dockerContainer, inspect dockerInspect, line string, maxBytes int) (map[string]any, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, false
	}
	var raw struct {
		Log    string `json:"log"`
		Stream string `json:"stream"`
		Time   string `json:"time"`
	}
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		raw.Log = line
	}
	message := strings.TrimRight(raw.Log, "\r\n")
	if message == "" {
		return nil, false
	}
	if maxBytes > 0 && len(message) > maxBytes {
		message = message[:maxBytes]
	}
	message, redactionStatus := redactContainerLogMessage(message)
	id := shortID(row.ID)
	event := map[string]any{
		"container_id":     id,
		"container_name":   firstName(row.Names),
		"image":            row.Image,
		"service":          composeService(row.Labels),
		"namespace":        containerNamespace(row.Labels),
		"stream":           strings.TrimSpace(raw.Stream),
		"timestamp_utc":    strings.TrimSpace(raw.Time),
		"message":          message,
		"redaction_status": redactionStatus,
		"transport":        "docker_json_file",
		"source_tool":      "docker",
		"source_category":  "container_log",
	}
	if user := strings.TrimSpace(inspect.Config.User); user != "" {
		event["user"] = user
	}
	applyOriginLabels(event, row.Labels)
	soclog.EnrichMap(event)
	return event, true
}

func applyOriginLabels(event map[string]any, labels map[string]string) {
	for label, field := range map[string]string{
		"aiceberg.ai/transport":        "aiceberg_transport",
		"aiceberg.ai/tool-origin":      "aiceberg_tool_origin",
		"aiceberg.ai/source-category":  "aiceberg_source_category",
		"aiceberg.ai/soc-source-type":  "aiceberg_soc_source_type",
		"aiceberg.ai/soc-eligible":     "aiceberg_soc_eligible",
		"aiceberg.ai/route-reason":     "aiceberg_route_reason",
		"aiceberg.com/transport":       "aiceberg_transport",
		"aiceberg.com/tool-origin":     "aiceberg_tool_origin",
		"aiceberg.com/source-category": "aiceberg_source_category",
		"aiceberg.com/soc-source-type": "aiceberg_soc_source_type",
		"aiceberg.com/soc-eligible":    "aiceberg_soc_eligible",
		"aiceberg.com/route-reason":    "aiceberg_route_reason",
	} {
		if value := strings.TrimSpace(labels[label]); value != "" {
			event[field] = value
			event["aiceberg_origin_confidence"] = "configured"
		}
	}
}

var containerSensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*:\s*(?:bearer|basic)\s+)[^\s]+`),
	regexp.MustCompile(`(?i)("(?:password|passwd|pwd|token|secret|api[_-]?key|cookie)"\s*:\s*)"[^"]*"`),
	regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|secret|api[_-]?key|cookie)\s*=\s*("[^"]+"|'[^']+'|[^\s&;]+)`),
	regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|secret|api[_-]?key|cookie)\s*:\s*("[^"]+"|'[^']+'|[^\s,}]+)`),
}

func redactContainerLogMessage(message string) (string, string) {
	redacted := message
	for _, pattern := range containerSensitivePatterns {
		redacted = pattern.ReplaceAllStringFunc(redacted, func(match string) string {
			if strings.HasPrefix(strings.TrimSpace(match), `"`) {
				if idx := strings.Index(match, ":"); idx >= 0 {
					return match[:idx+1] + `"[redacted]"`
				}
			}
			if idx := strings.Index(match, "="); idx >= 0 {
				return match[:idx+1] + "[redacted]"
			}
			if idx := strings.Index(match, ":"); idx >= 0 {
				prefix := match[:idx+1]
				if strings.Contains(strings.ToLower(prefix), "authorization") {
					parts := strings.Fields(match)
					if len(parts) >= 2 {
						return parts[0] + " " + parts[1] + " [redacted]"
					}
				}
				return prefix + "[redacted]"
			}
			return "[redacted]"
		})
	}
	if redacted != message {
		return redacted, "redacted"
	}
	return message, "none"
}

func loadLogCursor(path string) map[string]int64 {
	cursor := map[string]int64{}
	data, err := os.ReadFile(path)
	if err != nil {
		return cursor
	}
	_ = json.Unmarshal(data, &cursor)
	return cursor
}

func saveLogCursor(path string, cursor map[string]int64) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepathDir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(cursor)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func filepathDir(path string) string {
	idx := strings.LastIndexAny(path, `/\`)
	if idx <= 0 {
		return "."
	}
	return path[:idx]
}

func cpuPercent(stats dockerStats) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage)
	onlineCPUs := float64(len(stats.CPUStats.CPUUsage.PercpuUsage))
	if onlineCPUs <= 0 {
		onlineCPUs = 1
	}
	if cpuDelta <= 0 || systemDelta <= 0 {
		return 0
	}
	return (cpuDelta / systemDelta) * onlineCPUs * 100
}

func networkBytes(stats dockerStats) (uint64, uint64) {
	var rx, tx uint64
	for _, netStats := range stats.Networks {
		rx += netStats.RxBytes
		tx += netStats.TxBytes
	}
	return rx, tx
}

func blockBytes(stats dockerStats) (uint64, uint64) {
	var read, write uint64
	for _, row := range stats.BlkioStats.IoServiceBytesRecursive {
		switch strings.ToLower(row.Op) {
		case "read":
			read += row.Value
		case "write":
			write += row.Value
		}
	}
	return read, write
}

func networkNames(networks map[string]any) []string {
	out := make([]string, 0, len(networks))
	for name := range networks {
		out = append(out, name)
	}
	return out
}

func firstName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[0], "/")
}

func shortID(id string) string {
	id = strings.TrimPrefix(strings.TrimSpace(id), "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func intString(v int) string {
	if v <= 0 {
		return "0"
	}
	return strconv.Itoa(v)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
