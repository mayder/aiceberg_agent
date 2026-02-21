package networkcapture

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	gnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

const (
	defaultWindow   = 15 * time.Second
	defaultInterval = 1 * time.Second
	defaultMaxFlows = 1000
	defaultMaxPeers = 300
)

type collector struct {
	prefs    func() config.CollectPrefs
	window   time.Duration
	interval time.Duration
	maxFlows int
	maxPeers int
}

type capturePayload struct {
	Capture    captureMeta  `json:"capture"`
	Flows      []flowRow    `json:"flows,omitempty"`
	Peers      []peerRow    `json:"peers,omitempty"`
	Interfaces []ifaceDelta `json:"interfaces,omitempty"`
}

type captureMeta struct {
	Mode          string   `json:"mode"`
	Source        string   `json:"source"`
	WindowSec     int      `json:"window_sec"`
	SampleSeconds int      `json:"sample_seconds"`
	StartedAtUnix int64    `json:"started_at_unix"`
	EndedAtUnix   int64    `json:"ended_at_unix"`
	Hostname      string   `json:"hostname,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
}

type flowRow struct {
	Protocol    string `json:"protocol,omitempty"`
	Direction   string `json:"direction,omitempty"`
	LocalIP     string `json:"local_ip,omitempty"`
	LocalPort   uint32 `json:"local_port,omitempty"`
	RemoteIP    string `json:"remote_ip,omitempty"`
	RemotePort  uint32 `json:"remote_port,omitempty"`
	RemoteScope string `json:"remote_scope,omitempty"`
	State       string `json:"state,omitempty"`
	PID         int32  `json:"pid,omitempty"`
	Process     string `json:"process,omitempty"`
	Samples     int    `json:"samples"`
}

type peerRow struct {
	Protocol    string   `json:"protocol,omitempty"`
	Direction   string   `json:"direction,omitempty"`
	RemoteIP    string   `json:"remote_ip,omitempty"`
	RemotePort  uint32   `json:"remote_port,omitempty"`
	RemoteScope string   `json:"remote_scope,omitempty"`
	Samples     int      `json:"samples"`
	Processes   []string `json:"processes,omitempty"`
}

type ifaceDelta struct {
	Name        string `json:"name"`
	BytesSent   uint64 `json:"bytes_sent"`
	BytesRecv   uint64 `json:"bytes_recv"`
	PacketsSent uint64 `json:"packets_sent"`
	PacketsRecv uint64 `json:"packets_recv"`
	ErrIn       uint64 `json:"err_in"`
	ErrOut      uint64 `json:"err_out"`
	DropIn      uint64 `json:"drop_in"`
	DropOut     uint64 `json:"drop_out"`
}

type flowKey struct {
	proto      string
	direction  string
	localIP    string
	localPort  uint32
	remoteIP   string
	remotePort uint32
	pid        int32
	process    string
}

type flowAgg struct {
	state   string
	samples int
	scope   string
}

type peerKey struct {
	proto      string
	direction  string
	remoteIP   string
	remotePort uint32
	scope      string
}

type peerAgg struct {
	samples   int
	processes map[string]struct{}
}

func New(prefsProvider func() config.CollectPrefs) ports.Collector {
	return &collector{
		prefs:    prefsProvider,
		window:   defaultWindow,
		interval: defaultInterval,
		maxFlows: defaultMaxFlows,
		maxPeers: defaultMaxPeers,
	}
}

func (c *collector) Name() string { return "network_capture" }

func (c *collector) Interval() time.Duration { return 24 * time.Hour }

func (c *collector) Collect(ctx context.Context) ([]byte, error) {
	start := time.Now().UTC()
	end := start
	hostname, _ := os.Hostname()
	meta := captureMeta{
		Mode:          "passive",
		Source:        "socket_snapshot",
		WindowSec:     int(c.window.Seconds()),
		SampleSeconds: int(c.interval.Seconds()),
		StartedAtUnix: start.Unix(),
		Hostname:      hostname,
	}

	startIO, err := gnet.IOCountersWithContext(ctx, true)
	if err != nil {
		meta.Warnings = append(meta.Warnings, "network io counters indisponiveis no inicio")
	}

	flows := make(map[flowKey]*flowAgg)
	peers := make(map[peerKey]*peerAgg)
	procCache := make(map[int32]string)
	localIPs := localIPSet(ctx)

	sample := func() {
		conns, e := gnet.ConnectionsWithContext(ctx, "inet")
		if e != nil {
			meta.Warnings = append(meta.Warnings, "connections snapshot indisponivel")
			return
		}
		for _, conn := range conns {
			if conn.Raddr.IP == "" || isSpecialIP(conn.Raddr.IP) {
				continue
			}
			processName := ""
			if conn.Pid > 0 {
				if cached, ok := procCache[conn.Pid]; ok {
					processName = cached
				} else if p, e := process.NewProcessWithContext(ctx, conn.Pid); e == nil {
					if name, ne := p.NameWithContext(ctx); ne == nil {
						processName = strings.TrimSpace(name)
					}
					procCache[conn.Pid] = processName
				}
			}
			direction := inferDirection(conn.Status, conn.Laddr.Port, conn.Raddr.Port)
			scope := remoteScope(conn.Raddr.IP, localIPs)
			key := flowKey{
				proto:      protoName(conn.Type),
				direction:  direction,
				localIP:    conn.Laddr.IP,
				localPort:  conn.Laddr.Port,
				remoteIP:   conn.Raddr.IP,
				remotePort: conn.Raddr.Port,
				pid:        conn.Pid,
				process:    processName,
			}
			agg, ok := flows[key]
			if !ok {
				agg = &flowAgg{}
				flows[key] = agg
			}
			agg.samples++
			agg.state = conn.Status
			agg.scope = scope

			pk := peerKey{
				proto:      key.proto,
				direction:  key.direction,
				remoteIP:   key.remoteIP,
				remotePort: key.remotePort,
				scope:      scope,
			}
			peer, ok := peers[pk]
			if !ok {
				peer = &peerAgg{processes: make(map[string]struct{})}
				peers[pk] = peer
			}
			peer.samples++
			if processName != "" {
				peer.processes[processName] = struct{}{}
			}
		}
	}

	sample()
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	deadline := time.NewTimer(c.window)
	defer deadline.Stop()

	for {
		select {
		case <-ctx.Done():
			meta.Warnings = append(meta.Warnings, "coleta interrompida por contexto")
			end = time.Now().UTC()
			payload := capturePayload{
				Capture:    meta,
				Flows:      c.sortedFlows(flows),
				Peers:      c.sortedPeers(peers),
				Interfaces: ioDelta(startIO, nil),
			}
			payload.Capture.EndedAtUnix = end.Unix()
			return json.Marshal(payload)
		case <-deadline.C:
			end = time.Now().UTC()
			endIO, e := gnet.IOCountersWithContext(ctx, true)
			if e != nil {
				meta.Warnings = append(meta.Warnings, "network io counters indisponiveis no final")
			}
			meta.EndedAtUnix = end.Unix()
			payload := capturePayload{
				Capture:    meta,
				Flows:      c.sortedFlows(flows),
				Peers:      c.sortedPeers(peers),
				Interfaces: ioDelta(startIO, endIO),
			}
			return json.Marshal(payload)
		case <-ticker.C:
			sample()
		}
	}
}

func (c *collector) sortedFlows(flows map[flowKey]*flowAgg) []flowRow {
	out := make([]flowRow, 0, len(flows))
	for k, v := range flows {
		out = append(out, flowRow{
			Protocol:    k.proto,
			Direction:   k.direction,
			LocalIP:     k.localIP,
			LocalPort:   k.localPort,
			RemoteIP:    k.remoteIP,
			RemotePort:  k.remotePort,
			RemoteScope: v.scope,
			State:       v.state,
			PID:         k.pid,
			Process:     k.process,
			Samples:     v.samples,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Samples == out[j].Samples {
			if out[i].RemoteIP == out[j].RemoteIP {
				return out[i].RemotePort < out[j].RemotePort
			}
			return out[i].RemoteIP < out[j].RemoteIP
		}
		return out[i].Samples > out[j].Samples
	})
	if len(out) > c.maxFlows {
		out = out[:c.maxFlows]
	}
	return out
}

func (c *collector) sortedPeers(peers map[peerKey]*peerAgg) []peerRow {
	out := make([]peerRow, 0, len(peers))
	for k, v := range peers {
		processes := make([]string, 0, len(v.processes))
		for name := range v.processes {
			processes = append(processes, name)
		}
		sort.Strings(processes)
		if len(processes) > 5 {
			processes = processes[:5]
		}
		out = append(out, peerRow{
			Protocol:    k.proto,
			Direction:   k.direction,
			RemoteIP:    k.remoteIP,
			RemotePort:  k.remotePort,
			RemoteScope: k.scope,
			Samples:     v.samples,
			Processes:   processes,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Samples == out[j].Samples {
			if out[i].RemoteIP == out[j].RemoteIP {
				return out[i].RemotePort < out[j].RemotePort
			}
			return out[i].RemoteIP < out[j].RemoteIP
		}
		return out[i].Samples > out[j].Samples
	})
	if len(out) > c.maxPeers {
		out = out[:c.maxPeers]
	}
	return out
}

func ioDelta(start, end []gnet.IOCountersStat) []ifaceDelta {
	if len(start) == 0 || len(end) == 0 {
		return nil
	}
	startByName := make(map[string]gnet.IOCountersStat, len(start))
	for _, st := range start {
		startByName[st.Name] = st
	}
	var out []ifaceDelta
	for _, st := range end {
		base, ok := startByName[st.Name]
		if !ok {
			continue
		}
		out = append(out, ifaceDelta{
			Name:        st.Name,
			BytesSent:   safeDelta(st.BytesSent, base.BytesSent),
			BytesRecv:   safeDelta(st.BytesRecv, base.BytesRecv),
			PacketsSent: safeDelta(st.PacketsSent, base.PacketsSent),
			PacketsRecv: safeDelta(st.PacketsRecv, base.PacketsRecv),
			ErrIn:       safeDelta(st.Errin, base.Errin),
			ErrOut:      safeDelta(st.Errout, base.Errout),
			DropIn:      safeDelta(st.Dropin, base.Dropin),
			DropOut:     safeDelta(st.Dropout, base.Dropout),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func safeDelta(cur, prev uint64) uint64 {
	if cur < prev {
		return 0
	}
	return cur - prev
}

func localIPSet(ctx context.Context) map[string]struct{} {
	out := make(map[string]struct{})
	ifaces, err := gnet.InterfacesWithContext(ctx)
	if err != nil {
		return out
	}
	for _, inf := range ifaces {
		for _, addr := range inf.Addrs {
			host := addr.Addr
			if i := strings.Index(host, "/"); i >= 0 {
				host = host[:i]
			}
			host = strings.TrimSpace(host)
			if host != "" {
				out[host] = struct{}{}
			}
		}
	}
	return out
}

func remoteScope(remote string, localIPs map[string]struct{}) string {
	if remote == "" {
		return "unknown"
	}
	if _, ok := localIPs[remote]; ok {
		return "local_host"
	}
	ip := net.ParseIP(remote)
	if ip == nil {
		return "unknown"
	}
	if ip.IsLoopback() {
		return "loopback"
	}
	if ip.IsPrivate() {
		return "private"
	}
	if ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() {
		return "link_local"
	}
	return "public"
}

func isSpecialIP(ip string) bool {
	if ip == "" || ip == "0.0.0.0" || ip == "::" || ip == "*" {
		return true
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return parsed.IsLoopback() || parsed.IsUnspecified()
}

func inferDirection(state string, localPort, remotePort uint32) string {
	st := strings.ToUpper(strings.TrimSpace(state))
	switch st {
	case "SYN_SENT":
		return "egress"
	case "SYN_RECV":
		return "ingress"
	case "LISTEN":
		return "listen"
	}
	if localPort >= 49152 && remotePort < 49152 {
		return "egress"
	}
	if remotePort >= 49152 && localPort < 49152 {
		return "ingress"
	}
	if localPort == remotePort {
		return "lateral"
	}
	return "unknown"
}

func protoName(t uint32) string {
	switch t {
	case 1:
		return "tcp"
	case 2:
		return "udp"
	default:
		return "ip"
	}
}
