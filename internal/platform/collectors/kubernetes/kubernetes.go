package kubernetes

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

const schemaVersion = 1

type collector struct {
	prefs         func() config.CollectPrefs
	baseEnabled   bool
	apiURL        string
	tokenPath     string
	caPath        string
	nodeName      string
	namespace     string
	interval      time.Duration
	maxItems      int
	maxEvents     int
	logsEnabled   bool
	logCursorPath string
	logMaxLines   int
	logMaxBytes   int
	logInclude    string
	logExclude    string
}

func New(cfg config.Config, prefsProvider func() config.CollectPrefs) ports.Collector {
	interval := cfg.KubernetesInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	maxItems := cfg.KubernetesMaxItems
	if maxItems <= 0 {
		maxItems = 500
	}
	maxEvents := cfg.KubernetesMaxEvents
	if maxEvents <= 0 {
		maxEvents = 100
	}
	return &collector{
		prefs:         prefsProvider,
		baseEnabled:   cfg.KubernetesEnabled,
		apiURL:        strings.TrimRight(cfg.KubernetesAPIURL, "/"),
		tokenPath:     cfg.KubernetesTokenPath,
		caPath:        cfg.KubernetesCAPath,
		nodeName:      cfg.KubernetesNodeName,
		namespace:     cfg.KubernetesNamespace,
		interval:      interval,
		maxItems:      maxItems,
		maxEvents:     maxEvents,
		logsEnabled:   cfg.KubernetesLogsEnabled,
		logCursorPath: cfg.KubernetesLogsCursorPath,
		logMaxLines:   cfg.KubernetesLogsMaxLines,
		logMaxBytes:   cfg.KubernetesLogsMaxBytes,
		logInclude:    cfg.KubernetesLogsIncludeRegex,
		logExclude:    cfg.KubernetesLogsExcludeRegex,
	}
}

func (c *collector) Name() string { return "kubernetes" }

func (c *collector) Interval() time.Duration { return c.interval }

func (c *collector) Collect(ctx context.Context) ([]byte, error) {
	if !c.enabled() {
		return nil, nil
	}
	if strings.TrimSpace(c.apiURL) == "" {
		return nil, errors.New("kubernetes api url vazio")
	}
	token, err := os.ReadFile(c.tokenPath)
	if err != nil {
		return nil, err
	}
	client := kubernetesClient(c.caPath)
	pods, err := listPods(ctx, client, c.apiURL, string(token), c.namespace, c.nodeName, c.maxItems)
	if err != nil {
		return nil, err
	}
	nodes, err := listNodes(ctx, client, c.apiURL, string(token), c.maxItems)
	if err != nil {
		return nil, err
	}
	events, err := listEvents(ctx, client, c.apiURL, string(token), c.namespace, c.maxEvents)
	if err != nil {
		return nil, err
	}
	metrics := collectMetrics(ctx, client, c.apiURL, string(token), c.namespace)
	payload := map[string]any{
		"kubernetes": map[string]any{
			"schema_version":       schemaVersion,
			"source":               "kubernetes_api",
			"node_name":            c.nodeName,
			"namespace_scope":      c.namespace,
			"pods":                 normalizePods(pods),
			"nodes":                normalizeNodes(nodes),
			"events":               normalizeEvents(events),
			"metrics":              metrics,
			"logs":                 c.collectPodLogs(ctx, client, c.apiURL, string(token), pods),
			"autodiscovery_checks": autodiscoveryChecks(pods),
		},
	}
	return json.Marshal(payload)
}

func (c *collector) effectiveLogSettings() (bool, string, int, int, string, string) {
	enabled := c.logsEnabled
	cursorPath := c.logCursorPath
	maxLines := c.logMaxLines
	maxBytes := c.logMaxBytes
	include := c.logInclude
	exclude := c.logExclude
	if c.prefs != nil {
		p := c.prefs()
		if p.KubernetesLogsEnabled {
			enabled = true
		}
		if strings.TrimSpace(p.KubernetesLogsCursorPath) != "" {
			cursorPath = p.KubernetesLogsCursorPath
		}
		if p.KubernetesLogsMaxLines > 0 {
			maxLines = p.KubernetesLogsMaxLines
		}
		if p.KubernetesLogsMaxBytes > 0 {
			maxBytes = p.KubernetesLogsMaxBytes
		}
		if strings.TrimSpace(p.KubernetesLogsIncludeRegex) != "" {
			include = p.KubernetesLogsIncludeRegex
		}
		if strings.TrimSpace(p.KubernetesLogsExcludeRegex) != "" {
			exclude = p.KubernetesLogsExcludeRegex
		}
	}
	if maxLines <= 0 {
		maxLines = 200
	}
	if maxBytes <= 0 {
		maxBytes = 256 * 1024
	}
	return enabled, cursorPath, maxLines, maxBytes, include, exclude
}

func (c *collector) enabled() bool {
	p := config.CollectPrefs{}
	if c.prefs != nil {
		p = c.prefs()
	}
	if strings.TrimSpace(p.Version) == "" {
		return c.baseEnabled
	}
	return p.KubernetesEnabled
}

func kubernetesClient(caPath string) *http.Client {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if caPEM, err := os.ReadFile(caPath); err == nil && len(caPEM) > 0 {
		pool := x509.NewCertPool()
		if pool.AppendCertsFromPEM(caPEM) {
			tlsConfig.RootCAs = pool
		}
	}
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}
}

func listPods(ctx context.Context, client *http.Client, apiURL, token, namespace, nodeName string, limit int) ([]pod, error) {
	base := "/api/v1/pods"
	if strings.TrimSpace(namespace) != "" {
		base = "/api/v1/namespaces/" + url.PathEscape(namespace) + "/pods"
	}
	values := url.Values{}
	values.Set("limit", intString(limit))
	if strings.TrimSpace(nodeName) != "" {
		values.Set("fieldSelector", "spec.nodeName="+nodeName)
	}
	var out podList
	if err := getJSON(ctx, client, apiURL+base+"?"+values.Encode(), token, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

func listNodes(ctx context.Context, client *http.Client, apiURL, token string, limit int) ([]node, error) {
	var out nodeList
	if err := getJSON(ctx, client, apiURL+"/api/v1/nodes?limit="+url.QueryEscape(intString(limit)), token, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

func listEvents(ctx context.Context, client *http.Client, apiURL, token, namespace string, limit int) ([]event, error) {
	base := "/api/v1/events"
	if strings.TrimSpace(namespace) != "" {
		base = "/api/v1/namespaces/" + url.PathEscape(namespace) + "/events"
	}
	var out eventList
	if err := getJSON(ctx, client, apiURL+base+"?limit="+url.QueryEscape(intString(limit)), token, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

func collectMetrics(ctx context.Context, client *http.Client, apiURL, token, namespace string) map[string]any {
	nodeMetrics := nodeMetricsList{}
	if err := getJSON(ctx, client, apiURL+"/apis/metrics.k8s.io/v1beta1/nodes", token, &nodeMetrics); err != nil {
		nodeMetrics.Items = nil
	}
	podPath := "/apis/metrics.k8s.io/v1beta1/pods"
	if strings.TrimSpace(namespace) != "" {
		podPath = "/apis/metrics.k8s.io/v1beta1/namespaces/" + url.PathEscape(namespace) + "/pods"
	}
	podMetrics := podMetricsList{}
	if err := getJSON(ctx, client, apiURL+podPath, token, &podMetrics); err != nil {
		podMetrics.Items = nil
	}
	if len(nodeMetrics.Items) == 0 && len(podMetrics.Items) == 0 {
		return nil
	}
	return map[string]any{
		"nodes": normalizeNodeMetrics(nodeMetrics.Items),
		"pods":  normalizePodMetrics(podMetrics.Items),
	}
}

func getJSON(ctx context.Context, client *http.Client, rawURL, token string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("kubernetes api status " + resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func (c *collector) collectPodLogs(ctx context.Context, client *http.Client, apiURL, token string, pods []pod) map[string]any {
	enabled, cursorPath, maxLines, maxBytes, includeRegex, excludeRegex := c.effectiveLogSettings()
	if !enabled {
		return nil
	}
	include := compileRegex(includeRegex)
	exclude := compileRegex(excludeRegex)
	cursor := loadLogCursor(cursorPath)
	events := []map[string]any{}
	dropped := 0
	for _, p := range pods {
		for _, spec := range p.Spec.Containers {
			if len(events) >= maxLines {
				dropped++
				continue
			}
			if !matchesLogFilter(p, spec, include, exclude) {
				continue
			}
			read, drop := fetchPodContainerLogs(ctx, client, apiURL, token, p, spec, cursor, maxLines-len(events), maxBytes)
			events = append(events, read...)
			dropped += drop
		}
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

func fetchPodContainerLogs(ctx context.Context, client *http.Client, apiURL, token string, p pod, spec containerSpec, cursor map[string]string, maxLines, maxBytes int) ([]map[string]any, int) {
	if maxLines <= 0 {
		return nil, 0
	}
	key := podLogCursorKey(p, spec)
	values := url.Values{}
	values.Set("container", spec.Name)
	values.Set("timestamps", "true")
	values.Set("tailLines", intString(maxLines))
	if since := strings.TrimSpace(cursor[key]); since != "" {
		values.Set("sinceTime", since)
	}
	rawURL := strings.TrimRight(apiURL, "/") + "/api/v1/namespaces/" + url.PathEscape(p.Metadata.Namespace) + "/pods/" + url.PathEscape(p.Metadata.Name) + "/log?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 1
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	req.Header.Set("Accept", "text/plain")
	resp, err := client.Do(req)
	if err != nil {
		return nil, 1
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, 1
	}
	return readPodLogStream(io.LimitReader(resp.Body, int64(maxBytes)), p, spec, cursor, key, maxLines, maxBytes)
}

func readPodLogStream(reader io.Reader, p pod, spec containerSpec, cursor map[string]string, cursorKey string, maxLines, maxBytes int) ([]map[string]any, int) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), maxBytes)
	events := []map[string]any{}
	dropped := 0
	for scanner.Scan() {
		if len(events) >= maxLines {
			dropped++
			continue
		}
		event, timestamp, ok := parsePodLogLine(scanner.Text(), p, spec, maxBytes)
		if !ok {
			dropped++
			continue
		}
		events = append(events, event)
		if timestamp != "" {
			cursor[cursorKey] = timestamp
		}
	}
	return events, dropped
}

func parsePodLogLine(line string, p pod, spec containerSpec, maxBytes int) (map[string]any, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, "", false
	}
	timestamp, message := splitKubernetesTimestamp(line)
	if maxBytes > 0 && len(message) > maxBytes {
		message = message[:maxBytes]
	}
	message, redactionStatus := redactPodLogMessage(message)
	event := map[string]any{
		"namespace":        p.Metadata.Namespace,
		"pod":              p.Metadata.Name,
		"pod_uid":          p.Metadata.UID,
		"node_name":        p.Spec.NodeName,
		"container":        spec.Name,
		"image":            spec.Image,
		"timestamp_utc":    timestamp,
		"message":          message,
		"redaction_status": redactionStatus,
		"transport":        "kubernetes_pod_log",
		"source_tool":      "kubernetes",
		"source_category":  "pod_log",
	}
	return event, timestamp, true
}

func splitKubernetesTimestamp(line string) (string, string) {
	idx := strings.IndexByte(line, ' ')
	if idx <= 0 {
		return "", line
	}
	candidate := strings.TrimSpace(line[:idx])
	if _, err := time.Parse(time.RFC3339Nano, candidate); err == nil {
		return candidate, strings.TrimSpace(line[idx+1:])
	}
	return "", line
}

type podList struct {
	Items []pod `json:"items"`
}

type pod struct {
	Metadata metadata `json:"metadata"`
	Spec     struct {
		NodeName    string            `json:"nodeName"`
		Containers  []containerSpec   `json:"containers"`
		Init        []containerSpec   `json:"initContainers"`
		Volumes     []map[string]any  `json:"volumes"`
		Affinity    map[string]any    `json:"affinity"`
		Tolerations []map[string]any  `json:"tolerations"`
		Selector    map[string]string `json:"nodeSelector"`
	} `json:"spec"`
	Status struct {
		Phase             string            `json:"phase"`
		PodIP             string            `json:"podIP"`
		HostIP            string            `json:"hostIP"`
		ContainerStatuses []containerStatus `json:"containerStatuses"`
	} `json:"status"`
}

type nodeList struct {
	Items []node `json:"items"`
}

type node struct {
	Metadata metadata `json:"metadata"`
	Status   struct {
		Addresses []struct {
			Type    string `json:"type"`
			Address string `json:"address"`
		} `json:"addresses"`
		Capacity    map[string]string `json:"capacity"`
		Allocatable map[string]string `json:"allocatable"`
		Conditions  []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"conditions"`
		NodeInfo map[string]string `json:"nodeInfo"`
	} `json:"status"`
}

type eventList struct {
	Items []event `json:"items"`
}

type event struct {
	Metadata metadata `json:"metadata"`
	Type     string   `json:"type"`
	Reason   string   `json:"reason"`
	Message  string   `json:"message"`
	Source   struct {
		Component string `json:"component"`
		Host      string `json:"host"`
	} `json:"source"`
	InvolvedObject struct {
		Kind      string `json:"kind"`
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		UID       string `json:"uid"`
	} `json:"involvedObject"`
	Count          int    `json:"count"`
	FirstTimestamp string `json:"firstTimestamp"`
	LastTimestamp  string `json:"lastTimestamp"`
}

type nodeMetricsList struct {
	Items []nodeMetric `json:"items"`
}

type nodeMetric struct {
	Metadata  metadata          `json:"metadata"`
	Timestamp string            `json:"timestamp"`
	Window    string            `json:"window"`
	Usage     map[string]string `json:"usage"`
}

type podMetricsList struct {
	Items []podMetric `json:"items"`
}

type podMetric struct {
	Metadata   metadata               `json:"metadata"`
	Timestamp  string                 `json:"timestamp"`
	Window     string                 `json:"window"`
	Containers []containerMetricUsage `json:"containers"`
}

type containerMetricUsage struct {
	Name  string            `json:"name"`
	Usage map[string]string `json:"usage"`
}

type metadata struct {
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	UID             string            `json:"uid"`
	Labels          map[string]string `json:"labels"`
	Annotations     map[string]string `json:"annotations"`
	OwnerReferences []struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	} `json:"ownerReferences"`
}

type containerSpec struct {
	Name      string `json:"name"`
	Image     string `json:"image"`
	Resources struct {
		Requests map[string]string `json:"requests"`
		Limits   map[string]string `json:"limits"`
	} `json:"resources"`
	Ports []struct {
		Name          string `json:"name"`
		ContainerPort int    `json:"containerPort"`
		Protocol      string `json:"protocol"`
	} `json:"ports"`
}

type containerStatus struct {
	Name         string         `json:"name"`
	Ready        bool           `json:"ready"`
	RestartCount int            `json:"restartCount"`
	Image        string         `json:"image"`
	ImageID      string         `json:"imageID"`
	ContainerID  string         `json:"containerID"`
	State        map[string]any `json:"state"`
}

func normalizePods(rows []pod) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{
			"namespace":       row.Metadata.Namespace,
			"name":            row.Metadata.Name,
			"uid":             row.Metadata.UID,
			"node_name":       row.Spec.NodeName,
			"phase":           row.Status.Phase,
			"pod_ip":          row.Status.PodIP,
			"host_ip":         row.Status.HostIP,
			"labels":          safeMap(row.Metadata.Labels),
			"annotations":     safeAnnotations(row.Metadata.Annotations),
			"owner":           owner(row.Metadata.OwnerReferences),
			"containers":      normalizeContainerSpecs(row.Spec.Containers, row.Status.ContainerStatuses),
			"init_containers": normalizeContainerSpecs(row.Spec.Init, nil),
		}
		out = append(out, item)
	}
	return out
}

func normalizeNodes(rows []node) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"name":        row.Metadata.Name,
			"uid":         row.Metadata.UID,
			"labels":      safeMap(row.Metadata.Labels),
			"annotations": safeAnnotations(row.Metadata.Annotations),
			"addresses":   row.Status.Addresses,
			"capacity":    row.Status.Capacity,
			"allocatable": row.Status.Allocatable,
			"conditions":  row.Status.Conditions,
			"node_info":   row.Status.NodeInfo,
		})
	}
	return out
}

func normalizeEvents(rows []event) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"namespace":        row.Metadata.Namespace,
			"name":             row.Metadata.Name,
			"type":             row.Type,
			"reason":           row.Reason,
			"message":          truncate(row.Message, 500),
			"component":        row.Source.Component,
			"host":             row.Source.Host,
			"object_kind":      row.InvolvedObject.Kind,
			"object_namespace": row.InvolvedObject.Namespace,
			"object_name":      row.InvolvedObject.Name,
			"object_uid":       row.InvolvedObject.UID,
			"count":            row.Count,
			"first_timestamp":  row.FirstTimestamp,
			"last_timestamp":   row.LastTimestamp,
		})
	}
	return out
}

func normalizeNodeMetrics(rows []nodeMetric) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"name":       row.Metadata.Name,
			"timestamp":  row.Timestamp,
			"window":     row.Window,
			"cpu":        row.Usage["cpu"],
			"memory":     row.Usage["memory"],
			"usage":      row.Usage,
			"metric_src": "metrics.k8s.io",
		})
	}
	return out
}

func normalizePodMetrics(rows []podMetric) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		containers := make([]map[string]any, 0, len(row.Containers))
		for _, c := range row.Containers {
			containers = append(containers, map[string]any{
				"name":   c.Name,
				"cpu":    c.Usage["cpu"],
				"memory": c.Usage["memory"],
				"usage":  c.Usage,
			})
		}
		out = append(out, map[string]any{
			"namespace":  row.Metadata.Namespace,
			"name":       row.Metadata.Name,
			"timestamp":  row.Timestamp,
			"window":     row.Window,
			"containers": containers,
			"metric_src": "metrics.k8s.io",
		})
	}
	return out
}

func normalizeContainerSpecs(specs []containerSpec, statuses []containerStatus) []map[string]any {
	statusByName := map[string]containerStatus{}
	for _, status := range statuses {
		statusByName[status.Name] = status
	}
	out := make([]map[string]any, 0, len(specs))
	for _, spec := range specs {
		item := map[string]any{
			"name":     spec.Name,
			"image":    spec.Image,
			"requests": spec.Resources.Requests,
			"limits":   spec.Resources.Limits,
			"ports":    spec.Ports,
		}
		if status, ok := statusByName[spec.Name]; ok {
			item["ready"] = status.Ready
			item["restart_count"] = status.RestartCount
			item["image_id"] = status.ImageID
			item["container_id"] = shortContainerID(status.ContainerID)
			item["state"] = status.State
		}
		out = append(out, item)
	}
	return out
}

func autodiscoveryChecks(pods []pod) []map[string]any {
	out := []map[string]any{}
	for _, p := range pods {
		if raw := strings.TrimSpace(p.Metadata.Annotations["aiceberg.ai/checks"]); raw != "" {
			var checks []map[string]any
			if json.Unmarshal([]byte(raw), &checks) == nil {
				for _, check := range checks {
					check["namespace"] = p.Metadata.Namespace
					check["pod"] = p.Metadata.Name
					normalizePodAutodiscoveryCheck(check, p)
					out = append(out, check)
				}
			}
		}
		for key, value := range p.Metadata.Annotations {
			if !strings.HasPrefix(key, "aiceberg.ai/check.") {
				continue
			}
			out = append(out, map[string]any{
				"namespace": p.Metadata.Namespace,
				"pod":       p.Metadata.Name,
				"key":       strings.TrimPrefix(key, "aiceberg.ai/check."),
				"value":     value,
			})
			normalizePodAutodiscoveryCheck(out[len(out)-1], p)
		}
	}
	return out
}

func normalizePodAutodiscoveryCheck(check map[string]any, p pod) {
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
		target = strings.ReplaceAll(target, "%%host%%", firstPodHost(p))
	}
	check["target"] = target
	if _, ok := check["tags"]; !ok {
		tags := []string{}
		if p.Metadata.Namespace != "" {
			tags = append(tags, "namespace:"+p.Metadata.Namespace)
		}
		if p.Metadata.Name != "" {
			tags = append(tags, "pod:"+p.Metadata.Name)
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

func firstPodHost(p pod) string {
	if p.Status.PodIP != "" {
		return p.Status.PodIP
	}
	if p.Metadata.Name != "" {
		return p.Metadata.Name
	}
	return "127.0.0.1"
}

func firstNonEmpty(values ...any) any {
	for _, value := range values {
		if strings.TrimSpace(fmt.Sprint(value)) != "" && fmt.Sprint(value) != "<nil>" {
			return value
		}
	}
	return ""
}

func matchesLogFilter(p pod, spec containerSpec, include, exclude *regexp.Regexp) bool {
	target := podLogFilterText(p, spec)
	if include != nil && !include.MatchString(target) {
		return false
	}
	if exclude != nil && exclude.MatchString(target) {
		return false
	}
	return true
}

func podLogFilterText(p pod, spec containerSpec) string {
	parts := []string{p.Metadata.Namespace, p.Metadata.Name, p.Metadata.UID, p.Spec.NodeName, spec.Name, spec.Image}
	for key, value := range p.Metadata.Labels {
		parts = append(parts, key+"="+value)
	}
	for key, value := range p.Metadata.Annotations {
		if strings.HasPrefix(key, "aiceberg.ai/") {
			parts = append(parts, key+"="+value)
		}
	}
	return strings.Join(parts, " ")
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

var podLogSensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*:\s*(?:bearer|basic)\s+)[^\s]+`),
	regexp.MustCompile(`(?i)("(?:password|passwd|pwd|token|secret|api[_-]?key|cookie)"\s*:\s*)"[^"]*"`),
	regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|secret|api[_-]?key|cookie)\s*=\s*("[^"]+"|'[^']+'|[^\s&;]+)`),
	regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|secret|api[_-]?key|cookie)\s*:\s*("[^"]+"|'[^']+'|[^\s,}]+)`),
}

func redactPodLogMessage(message string) (string, string) {
	redacted := message
	for _, pattern := range podLogSensitivePatterns {
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

func podLogCursorKey(p pod, spec containerSpec) string {
	return p.Metadata.Namespace + "/" + p.Metadata.Name + "/" + spec.Name
}

func loadLogCursor(path string) map[string]string {
	cursor := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return cursor
	}
	_ = json.Unmarshal(data, &cursor)
	return cursor
}

func saveLogCursor(path string, cursor map[string]string) error {
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

func safeMap(values map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range values {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "password") {
			out[key] = "[redacted]"
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func safeAnnotations(values map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range values {
		if strings.HasPrefix(key, "kubectl.kubernetes.io/last-applied-configuration") {
			continue
		}
		lower := strings.ToLower(key)
		if strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "password") {
			out[key] = "[redacted]"
			continue
		}
		out[key] = truncate(value, 1000)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func owner(refs []struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}) map[string]string {
	if len(refs) == 0 {
		return nil
	}
	return map[string]string{"kind": refs[0].Kind, "name": refs[0].Name}
}

func shortContainerID(value string) string {
	parts := strings.Split(value, "://")
	if len(parts) == 2 {
		value = parts[1]
	}
	return shortID(value)
}

func shortID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 12 {
		return value[:12]
	}
	return value
}

func truncate(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}

func intString(v int) string {
	if v <= 0 {
		return "1"
	}
	return strconv.Itoa(v)
}
