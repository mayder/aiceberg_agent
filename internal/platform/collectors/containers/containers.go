package containers

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
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
	socket      string
	interval    time.Duration
	maxItems    int
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
		prefs:       prefsProvider,
		baseEnabled: cfg.ContainerEnabled,
		socket:      cfg.ContainerDockerSocket,
		interval:    interval,
		maxItems:    maxItems,
	}
}

func (c *collector) Name() string { return "containers" }

func (c *collector) Interval() time.Duration { return c.interval }

func (c *collector) Collect(ctx context.Context) ([]byte, error) {
	if !c.enabled() {
		return nil, nil
	}
	if strings.TrimSpace(c.socket) == "" {
		return nil, errors.New("docker socket vazio")
	}
	client := dockerClient(c.socket)
	containers, err := listDockerContainers(ctx, client, c.maxItems)
	if err != nil {
		return nil, err
	}
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
			"schema_version": schemaVersion,
			"source":         "docker_socket",
			"items":          normalizeContainers(containers, statsByID),
			"dropped_count":  maxInt(0, len(containers)-c.maxItems),
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

func normalizeContainers(rows []dockerContainer, statsByID map[string]dockerStats) []map[string]any {
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
