package kubernetes

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

const schemaVersion = 1

type collector struct {
	prefs       func() config.CollectPrefs
	baseEnabled bool
	apiURL      string
	tokenPath   string
	caPath      string
	nodeName    string
	namespace   string
	interval    time.Duration
	maxItems    int
	maxEvents   int
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
		prefs:       prefsProvider,
		baseEnabled: cfg.KubernetesEnabled,
		apiURL:      strings.TrimRight(cfg.KubernetesAPIURL, "/"),
		tokenPath:   cfg.KubernetesTokenPath,
		caPath:      cfg.KubernetesCAPath,
		nodeName:    cfg.KubernetesNodeName,
		namespace:   cfg.KubernetesNamespace,
		interval:    interval,
		maxItems:    maxItems,
		maxEvents:   maxEvents,
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
	payload := map[string]any{
		"kubernetes": map[string]any{
			"schema_version":       schemaVersion,
			"source":               "kubernetes_api",
			"node_name":            c.nodeName,
			"namespace_scope":      c.namespace,
			"pods":                 normalizePods(pods),
			"nodes":                normalizeNodes(nodes),
			"events":               normalizeEvents(events),
			"autodiscovery_checks": autodiscoveryChecks(pods),
		},
	}
	return json.Marshal(payload)
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
		}
	}
	return out
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
