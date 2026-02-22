package networkcapture

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	gnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

const (
	defaultWindow       = 15 * time.Second
	defaultInterval     = 1 * time.Second
	defaultMaxFlows     = 1000
	defaultMaxPeers     = 300
	defaultMaxListeners = 300
	minWindow           = 5 * time.Second
	maxWindow           = 5 * time.Minute
	minInterval         = 1 * time.Second
	maxInterval         = 30 * time.Second
	minMaxRows          = 50
	maxMaxRows          = 5000
	defaultPCAPDuration = 3 * time.Second
	defaultPCAPPackets  = 160
	maxPCAPDuration     = 15 * time.Second
	maxPCAPPackets      = 2000
	defaultExternalMax  = 1000
	maxExternalMax      = 10000
	maxCmdlineLength    = 240
	maxResolutionLen    = 255
	maxTLSSubjectLen    = 512
	maxServiceNameLen   = 120
	reverseDNSCacheTTL  = 10 * time.Minute
	reverseDNSFailTTL   = 2 * time.Minute
	reverseDNSTimeout   = 450 * time.Millisecond
	maxReverseDNSCache  = 4096
)

var highRiskPorts = map[uint32]struct{}{
	21: {}, 22: {}, 23: {}, 25: {}, 53: {}, 110: {}, 135: {}, 137: {}, 138: {}, 139: {},
	143: {}, 389: {}, 445: {}, 1433: {}, 1521: {}, 3306: {}, 3389: {}, 5432: {}, 5900: {},
	6379: {}, 8080: {}, 9200: {}, 11211: {}, 27017: {},
}

var legacyPlaintextPorts = map[uint32]struct{}{
	21: {}, 23: {}, 110: {}, 143: {}, 389: {},
}

var adminPorts = map[uint32]struct{}{
	22: {}, 23: {}, 139: {}, 445: {}, 1433: {}, 1521: {}, 2375: {}, 2376: {}, 3306: {},
	3389: {}, 5432: {}, 5985: {}, 5986: {}, 6443: {}, 27017: {},
}

var dnsServicePorts = map[uint32]struct{}{
	53: {}, 853: {},
}

var handshakeNoSessionStates = map[string]struct{}{
	"SYN_SENT": {}, "SYN_RECV": {}, "SYN_RCVD": {}, "FIN_WAIT1": {}, "FIN_WAIT2": {}, "CLOSING": {},
}

var tlsLikelyPorts = map[uint32]struct{}{
	443: {}, 465: {}, 587: {}, 636: {}, 853: {}, 993: {}, 995: {}, 8443: {}, 9443: {},
}

var urlHostRegex = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+\-.]*://([a-z0-9\.\-]+)`)
var tcpdumpLineRegex = regexp.MustCompile(`^(IP6?)\s+(.+?)\s+>\s+(.+?):\s*(.*)$`)

type collector struct {
	prefs        func() config.CollectPrefs
	window       time.Duration
	interval     time.Duration
	maxFlows     int
	maxPeers     int
	maxListeners int
}

type capturePayload struct {
	Capture         captureMeta             `json:"capture"`
	Flows           []flowRow               `json:"flows,omitempty"`
	SocketSnapshot  []socketSnapshotRow     `json:"socket_snapshot,omitempty"`
	Peers           []peerRow               `json:"peers,omitempty"`
	Listeners       []listenerRow           `json:"listeners,omitempty"`
	Interfaces      []ifaceDelta            `json:"interfaces,omitempty"`
	LocalCtx        *localContext           `json:"local_context,omitempty"`
	PassiveAdvanced *passiveAdvancedPayload `json:"passive_advanced,omitempty"`
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

type passiveAdvancedPayload struct {
	RequestedMode   string                 `json:"requested_mode,omitempty"`
	AppliedMode     string                 `json:"applied_mode,omitempty"`
	Sources         []string               `json:"sources,omitempty"`
	Capabilities    passiveCapabilities    `json:"capabilities"`
	NetlinkLinks    []netlinkLinkRow       `json:"netlink_links,omitempty"`
	PCAP            *pcapSummary           `json:"pcap,omitempty"`
	ExternalSources []externalSourceStatus `json:"external_sources,omitempty"`
}

type passiveCapabilities struct {
	Netlink bool `json:"netlink"`
	PCAP    bool `json:"pcap"`
	EBPF    bool `json:"ebpf"`
}

type netlinkLinkRow struct {
	Name         string `json:"name,omitempty"`
	OperState    string `json:"oper_state,omitempty"`
	HardwareAddr string `json:"hardware_addr,omitempty"`
	MTU          int    `json:"mtu,omitempty"`
	RXBytes      uint64 `json:"rx_bytes,omitempty"`
	RXPackets    uint64 `json:"rx_packets,omitempty"`
	RXErrors     uint64 `json:"rx_errors,omitempty"`
	RXDrops      uint64 `json:"rx_drops,omitempty"`
	TXBytes      uint64 `json:"tx_bytes,omitempty"`
	TXPackets    uint64 `json:"tx_packets,omitempty"`
	TXErrors     uint64 `json:"tx_errors,omitempty"`
	TXDrops      uint64 `json:"tx_drops,omitempty"`
}

type pcapSummary struct {
	Interface         string        `json:"interface,omitempty"`
	RequestedDuration int           `json:"requested_duration_sec,omitempty"`
	RequestedPackets  int           `json:"requested_packets,omitempty"`
	CapturedPackets   int           `json:"captured_packets,omitempty"`
	ReceivedByFilter  int           `json:"received_by_filter,omitempty"`
	DroppedByKernel   int           `json:"dropped_by_kernel,omitempty"`
	Flows             []pcapFlowRow `json:"flows,omitempty"`
}

type pcapFlowRow struct {
	Protocol   string `json:"protocol,omitempty"`
	Direction  string `json:"direction,omitempty"`
	LocalIP    string `json:"local_ip,omitempty"`
	LocalPort  uint32 `json:"local_port,omitempty"`
	RemoteIP   string `json:"remote_ip,omitempty"`
	RemotePort uint32 `json:"remote_port,omitempty"`
	Samples    int    `json:"samples"`
}

type passiveCollectOptions struct {
	requestedMode string
	window        time.Duration
	interval      time.Duration
	maxFlows      int
	maxPeers      int
	maxListeners  int
	pcapEnabled   bool
	pcapIface     string
	pcapDuration  time.Duration
	pcapPackets   int
	externalSpecs []string
	externalMax   int
}

type externalSourceStatus struct {
	Type      string `json:"type,omitempty"`
	Path      string `json:"path,omitempty"`
	Parsed    int    `json:"parsed,omitempty"`
	Dropped   int    `json:"dropped,omitempty"`
	LastError string `json:"last_error,omitempty"`
}

type externalObservation struct {
	sourceType  string
	protocol    string
	direction   string
	state       string
	srcIP       string
	dstIP       string
	srcPort     uint32
	dstPort     uint32
	bytes       int
	packets     int
	firstSeen   int64
	lastSeen    int64
	dnsQuery    string
	dnsAnswer   string
	sni         string
	tlsSubject  string
	resetHits   int
	timeoutHits int
}

type flowRow struct {
	Protocol          string   `json:"protocol,omitempty"`
	Direction         string   `json:"direction,omitempty"`
	LocalIP           string   `json:"local_ip,omitempty"`
	LocalPort         uint32   `json:"local_port,omitempty"`
	RemoteIP          string   `json:"remote_ip,omitempty"`
	ReverseDNS        string   `json:"reverse_dns,omitempty"`
	DNSQuery          string   `json:"dns_query,omitempty"`
	DNSAnswer         string   `json:"dns_answer,omitempty"`
	SNI               string   `json:"sni,omitempty"`
	TLSSubject        string   `json:"tls_subject,omitempty"`
	FirstSeenUnix     int64    `json:"first_seen_unix,omitempty"`
	LastSeenUnix      int64    `json:"last_seen_unix,omitempty"`
	RemotePort        uint32   `json:"remote_port,omitempty"`
	RemoteScope       string   `json:"remote_scope,omitempty"`
	State             string   `json:"state,omitempty"`
	PID               int32    `json:"pid,omitempty"`
	ParentPID         int32    `json:"parent_pid,omitempty"`
	Process           string   `json:"process,omitempty"`
	ServiceName       string   `json:"service_name,omitempty"`
	ProcessUser       string   `json:"process_user,omitempty"`
	ProcessExe        string   `json:"process_exe,omitempty"`
	ProcessCmd        string   `json:"process_cmdline,omitempty"`
	BytesIn           int      `json:"bytes_in,omitempty"`
	BytesOut          int      `json:"bytes_out,omitempty"`
	PacketsIn         int      `json:"packets_in,omitempty"`
	PacketsOut        int      `json:"packets_out,omitempty"`
	ResetHits         int      `json:"reset_hits,omitempty"`
	TimeoutHits       int      `json:"timeout_hits,omitempty"`
	AvgDurationSec    float64  `json:"avg_duration_sec,omitempty"`
	IsProblematic     bool     `json:"is_problematic,omitempty"`
	ProblematicReason string   `json:"problematic_reason,omitempty"`
	RiskTags          []string `json:"problematic_tags,omitempty"`
	Samples           int      `json:"samples"`
}

type peerRow struct {
	Protocol          string   `json:"protocol,omitempty"`
	Direction         string   `json:"direction,omitempty"`
	RemoteIP          string   `json:"remote_ip,omitempty"`
	ReverseDNS        string   `json:"reverse_dns,omitempty"`
	DNSQuery          string   `json:"dns_query,omitempty"`
	DNSAnswer         string   `json:"dns_answer,omitempty"`
	SNI               string   `json:"sni,omitempty"`
	TLSSubject        string   `json:"tls_subject,omitempty"`
	FirstSeenUnix     int64    `json:"first_seen_unix,omitempty"`
	LastSeenUnix      int64    `json:"last_seen_unix,omitempty"`
	RemotePort        uint32   `json:"remote_port,omitempty"`
	RemoteScope       string   `json:"remote_scope,omitempty"`
	Samples           int      `json:"samples"`
	Processes         []string `json:"processes,omitempty"`
	ServiceNames      []string `json:"service_names,omitempty"`
	ProcessUsers      []string `json:"process_users,omitempty"`
	BytesIn           int      `json:"bytes_in,omitempty"`
	BytesOut          int      `json:"bytes_out,omitempty"`
	PacketsIn         int      `json:"packets_in,omitempty"`
	PacketsOut        int      `json:"packets_out,omitempty"`
	ResetHits         int      `json:"reset_hits,omitempty"`
	TimeoutHits       int      `json:"timeout_hits,omitempty"`
	AvgDurationSec    float64  `json:"avg_duration_sec,omitempty"`
	IsProblematic     bool     `json:"is_problematic,omitempty"`
	ProblematicReason string   `json:"problematic_reason,omitempty"`
	RiskTags          []string `json:"problematic_tags,omitempty"`
}

type listenerRow struct {
	Protocol    string `json:"protocol,omitempty"`
	LocalIP     string `json:"local_ip,omitempty"`
	LocalPort   uint32 `json:"local_port,omitempty"`
	State       string `json:"state,omitempty"`
	PID         int32  `json:"pid,omitempty"`
	ParentPID   int32  `json:"parent_pid,omitempty"`
	Process     string `json:"process,omitempty"`
	ServiceName string `json:"service_name,omitempty"`
	ProcessUser string `json:"process_user,omitempty"`
	ProcessExe  string `json:"process_exe,omitempty"`
	ProcessCmd  string `json:"process_cmdline,omitempty"`
	Samples     int    `json:"samples"`
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

type localContext struct {
	CapturedAtUnix int64           `json:"captured_at_unix,omitempty"`
	Hostname       string          `json:"hostname,omitempty"`
	DefaultGateway string          `json:"default_gateway,omitempty"`
	DNSServers     []string        `json:"dns_servers,omitempty"`
	Interfaces     []localIfaceRow `json:"interfaces,omitempty"`
	Routes         []localRouteRow `json:"routes,omitempty"`
	Neighbors      []localNeighRow `json:"neighbors,omitempty"`
}

type localIfaceRow struct {
	Name         string   `json:"name,omitempty"`
	Mtu          int      `json:"mtu,omitempty"`
	HardwareAddr string   `json:"hardware_addr,omitempty"`
	IsUp         bool     `json:"is_up,omitempty"`
	Flags        []string `json:"flags,omitempty"`
	Addrs        []string `json:"addrs,omitempty"`
}

type localRouteRow struct {
	Interface     string `json:"interface,omitempty"`
	Destination   string `json:"destination,omitempty"`
	Mask          string `json:"mask,omitempty"`
	Gateway       string `json:"gateway,omitempty"`
	Metric        int    `json:"metric,omitempty"`
	IsDefault     bool   `json:"is_default,omitempty"`
	Protocol      string `json:"protocol,omitempty"`
	AddressFamily string `json:"address_family,omitempty"`
}

type localNeighRow struct {
	IP            string `json:"ip,omitempty"`
	MAC           string `json:"mac,omitempty"`
	Interface     string `json:"interface,omitempty"`
	State         string `json:"state,omitempty"`
	AddressFamily string `json:"address_family,omitempty"`
}

type flowKey struct {
	proto       string
	direction   string
	localIP     string
	localPort   uint32
	remoteIP    string
	remotePort  uint32
	pid         int32
	parentPID   int32
	process     string
	serviceName string
	processUser string
	processExe  string
	processCmd  string
}

type flowAgg struct {
	state       string
	samples     int
	scope       string
	reverseDNS  string
	dnsQuery    string
	dnsAnswer   string
	sni         string
	tlsSubject  string
	firstSeen   int64
	lastSeen    int64
	bytesIn     int
	bytesOut    int
	packetsIn   int
	packetsOut  int
	resetHits   int
	timeoutHits int
	problematic bool
	reasons     map[string]struct{}
	tags        map[string]struct{}
}

type socketSnapshotRow struct {
	Protocol       string  `json:"protocol,omitempty"`
	Direction      string  `json:"direction,omitempty"`
	LocalIP        string  `json:"local_ip,omitempty"`
	LocalPort      uint32  `json:"local_port,omitempty"`
	RemoteIP       string  `json:"remote_ip,omitempty"`
	RemotePort     uint32  `json:"remote_port,omitempty"`
	DNSQuery       string  `json:"dns_query,omitempty"`
	DNSAnswer      string  `json:"dns_answer,omitempty"`
	SNI            string  `json:"sni,omitempty"`
	TLSSubject     string  `json:"tls_subject,omitempty"`
	State          string  `json:"state,omitempty"`
	BytesIn        int     `json:"bytes_in,omitempty"`
	BytesOut       int     `json:"bytes_out,omitempty"`
	PacketsIn      int     `json:"packets_in,omitempty"`
	PacketsOut     int     `json:"packets_out,omitempty"`
	ResetHits      int     `json:"reset_hits,omitempty"`
	TimeoutHits    int     `json:"timeout_hits,omitempty"`
	AvgDurationSec float64 `json:"avg_duration_sec,omitempty"`
	FirstSeenUnix  int64   `json:"first_seen_unix,omitempty"`
	LastSeenUnix   int64   `json:"last_seen_unix,omitempty"`
	Samples        int     `json:"samples"`
}

type socketSnapshotKey struct {
	proto      string
	direction  string
	localIP    string
	localPort  uint32
	remoteIP   string
	remotePort uint32
	state      string
}

type socketSnapshotAgg struct {
	samples          int
	firstSeen        int64
	lastSeen         int64
	dnsQuery         string
	dnsAnswer        string
	sni              string
	tlsSubject       string
	bytesIn          int
	bytesOut         int
	packetsIn        int
	packetsOut       int
	resetHits        int
	timeoutHits      int
	durationWeighted float64
	durationWeight   float64
}

type peerKey struct {
	proto      string
	direction  string
	remoteIP   string
	remotePort uint32
	scope      string
}

type peerAgg struct {
	samples      int
	reverseDNS   string
	dnsQuery     string
	dnsAnswer    string
	sni          string
	tlsSubject   string
	firstSeen    int64
	lastSeen     int64
	bytesIn      int
	bytesOut     int
	packetsIn    int
	packetsOut   int
	resetHits    int
	timeoutHits  int
	processes    map[string]struct{}
	serviceNames map[string]struct{}
	processUsers map[string]struct{}
	problematic  bool
	reasons      map[string]struct{}
	tags         map[string]struct{}
}

type reverseDNSCacheEntry struct {
	host      string
	expiresAt time.Time
}

var reverseDNSCache = struct {
	mu      sync.Mutex
	entries map[string]reverseDNSCacheEntry
}{
	entries: make(map[string]reverseDNSCacheEntry),
}

type listenerKey struct {
	proto       string
	localIP     string
	localPort   uint32
	state       string
	pid         int32
	parentPID   int32
	process     string
	serviceName string
	processUser string
	processExe  string
	processCmd  string
}

type listenerAgg struct {
	samples int
}

type processMeta struct {
	name     string
	service  string
	username string
	exe      string
	cmdline  string
	parentID int32
}

func New(prefsProvider func() config.CollectPrefs) ports.Collector {
	return &collector{
		prefs:        prefsProvider,
		window:       defaultWindow,
		interval:     defaultInterval,
		maxFlows:     defaultMaxFlows,
		maxPeers:     defaultMaxPeers,
		maxListeners: defaultMaxListeners,
	}
}

func (c *collector) Name() string { return "network_capture" }

func (c *collector) Interval() time.Duration { return 24 * time.Hour }

func (c *collector) Collect(ctx context.Context) ([]byte, error) {
	options := c.resolveOptions()
	start := time.Now().UTC()
	end := start
	hostname, _ := os.Hostname()
	meta := captureMeta{
		Mode:          "passive",
		Source:        "socket_snapshot",
		WindowSec:     int(options.window.Seconds()),
		SampleSeconds: int(options.interval.Seconds()),
		StartedAtUnix: start.Unix(),
		Hostname:      hostname,
	}
	advanced := &passiveAdvancedPayload{
		RequestedMode: options.requestedMode,
		Capabilities:  detectPassiveCapabilities(),
		Sources:       []string{"socket_snapshot"},
	}

	startIO, err := gnet.IOCountersWithContext(ctx, true)
	if err != nil {
		meta.Warnings = append(meta.Warnings, "network io counters indisponiveis no inicio")
	}
	localCtx, localCtxWarnings := collectLocalContext(ctx, hostname)
	if len(localCtxWarnings) > 0 {
		meta.Warnings = append(meta.Warnings, localCtxWarnings...)
	}

	flows := make(map[flowKey]*flowAgg)
	peers := make(map[peerKey]*peerAgg)
	listeners := make(map[listenerKey]*listenerAgg)
	procCache := make(map[int32]processMeta)
	captureReverseDNSCache := make(map[string]string)
	localIPs := localIPSet(ctx)

	sample := func() {
		sampleUnix := time.Now().UTC().Unix()
		conns, e := gnet.ConnectionsWithContext(ctx, "inet")
		if e != nil {
			meta.Warnings = append(meta.Warnings, "connections snapshot indisponivel")
			return
		}
		for _, conn := range conns {
			proc := processMeta{}
			if conn.Pid > 0 {
				if cached, ok := procCache[conn.Pid]; ok {
					proc = cached
				} else {
					proc = resolveProcessMeta(ctx, conn.Pid)
					procCache[conn.Pid] = proc
				}
			}

			if conn.Raddr.IP == "" {
				if conn.Status == "LISTEN" || conn.Raddr.Port == 0 {
					lk := listenerKey{
						proto:       protoName(conn.Type),
						localIP:     conn.Laddr.IP,
						localPort:   conn.Laddr.Port,
						state:       conn.Status,
						pid:         conn.Pid,
						parentPID:   proc.parentID,
						process:     proc.name,
						serviceName: proc.service,
						processUser: proc.username,
						processExe:  proc.exe,
						processCmd:  proc.cmdline,
					}
					agg, ok := listeners[lk]
					if !ok {
						agg = &listenerAgg{}
						listeners[lk] = agg
					}
					agg.samples++
				}
				continue
			}

			if isSpecialIP(conn.Raddr.IP) {
				continue
			}

			proto := protoName(conn.Type)
			direction := inferDirection(conn.Status, conn.Laddr.Port, conn.Raddr.Port)
			scope := remoteScope(conn.Raddr.IP, localIPs)
			reverseDNS, ok := captureReverseDNSCache[conn.Raddr.IP]
			if !ok {
				reverseDNS = resolveReverseDNSWithCache(ctx, conn.Raddr.IP)
				captureReverseDNSCache[conn.Raddr.IP] = reverseDNS
			}
			dnsQuery, dnsAnswer, sni, tlsSubject := inferResolutionContext(proc.cmdline, reverseDNS, conn.Raddr.IP, conn.Raddr.Port, proto)
			baseRisk := classifyNetworkRisk(conn.Raddr.Port, scope, direction, proto)
			key := flowKey{
				proto:       proto,
				direction:   direction,
				localIP:     conn.Laddr.IP,
				localPort:   conn.Laddr.Port,
				remoteIP:    conn.Raddr.IP,
				remotePort:  conn.Raddr.Port,
				pid:         conn.Pid,
				parentPID:   proc.parentID,
				process:     proc.name,
				serviceName: proc.service,
				processUser: proc.username,
				processExe:  proc.exe,
				processCmd:  proc.cmdline,
			}
			agg, ok := flows[key]
			if !ok {
				agg = &flowAgg{
					reasons: make(map[string]struct{}),
					tags:    make(map[string]struct{}),
				}
				flows[key] = agg
			}
			agg.samples++
			if agg.firstSeen == 0 || sampleUnix < agg.firstSeen {
				agg.firstSeen = sampleUnix
			}
			if sampleUnix > agg.lastSeen {
				agg.lastSeen = sampleUnix
			}
			if isResetLikeState(conn.Status) {
				agg.resetHits++
			}
			if isTimeoutLikeState(conn.Status) {
				agg.timeoutHits++
			}
			agg.state = conn.Status
			agg.scope = scope
			if agg.reverseDNS == "" && reverseDNS != "" {
				agg.reverseDNS = reverseDNS
			}
			if agg.dnsQuery == "" && dnsQuery != "" {
				agg.dnsQuery = dnsQuery
			}
			if agg.dnsAnswer == "" && dnsAnswer != "" {
				agg.dnsAnswer = dnsAnswer
			}
			if agg.sni == "" && sni != "" {
				agg.sni = sni
			}
			if agg.tlsSubject == "" && tlsSubject != "" {
				agg.tlsSubject = tlsSubject
			}
			mergeNetworkRisk(&agg.problematic, agg.reasons, agg.tags, baseRisk)
			flowFailureRisk := classifyNetworkFailureRisk(
				conn.Raddr.Port,
				scope,
				direction,
				proto,
				agg.state,
				agg.dnsQuery,
				agg.dnsAnswer,
				agg.samples,
				agg.resetHits,
				agg.timeoutHits,
			)
			mergeNetworkRisk(&agg.problematic, agg.reasons, agg.tags, flowFailureRisk)

			pk := peerKey{
				proto:      key.proto,
				direction:  key.direction,
				remoteIP:   key.remoteIP,
				remotePort: key.remotePort,
				scope:      scope,
			}
			peer, ok := peers[pk]
			if !ok {
				peer = &peerAgg{
					processes:    make(map[string]struct{}),
					serviceNames: make(map[string]struct{}),
					processUsers: make(map[string]struct{}),
					reasons:      make(map[string]struct{}),
					tags:         make(map[string]struct{}),
				}
				peers[pk] = peer
			}
			peer.samples++
			if peer.firstSeen == 0 || sampleUnix < peer.firstSeen {
				peer.firstSeen = sampleUnix
			}
			if sampleUnix > peer.lastSeen {
				peer.lastSeen = sampleUnix
			}
			if isResetLikeState(conn.Status) {
				peer.resetHits++
			}
			if isTimeoutLikeState(conn.Status) {
				peer.timeoutHits++
			}
			if peer.reverseDNS == "" && reverseDNS != "" {
				peer.reverseDNS = reverseDNS
			}
			if peer.dnsQuery == "" && dnsQuery != "" {
				peer.dnsQuery = dnsQuery
			}
			if peer.dnsAnswer == "" && dnsAnswer != "" {
				peer.dnsAnswer = dnsAnswer
			}
			if peer.sni == "" && sni != "" {
				peer.sni = sni
			}
			if peer.tlsSubject == "" && tlsSubject != "" {
				peer.tlsSubject = tlsSubject
			}
			mergeNetworkRisk(&peer.problematic, peer.reasons, peer.tags, baseRisk)
			peerFailureRisk := classifyNetworkFailureRisk(
				conn.Raddr.Port,
				scope,
				direction,
				proto,
				conn.Status,
				peer.dnsQuery,
				peer.dnsAnswer,
				peer.samples,
				peer.resetHits,
				peer.timeoutHits,
			)
			mergeNetworkRisk(&peer.problematic, peer.reasons, peer.tags, peerFailureRisk)
			if proc.name != "" {
				peer.processes[proc.name] = struct{}{}
			}
			if proc.service != "" {
				peer.serviceNames[proc.service] = struct{}{}
			}
			if proc.username != "" {
				peer.processUsers[proc.username] = struct{}{}
			}
		}
	}

	sample()
	ticker := time.NewTicker(options.interval)
	defer ticker.Stop()
	deadline := time.NewTimer(options.window)
	defer deadline.Stop()

	finalize := func(deltas []ifaceDelta) ([]byte, error) {
		externalSources := mergeExternalSources(options, localIPs, flows, peers, &meta)
		if len(externalSources) > 0 {
			advanced.ExternalSources = externalSources
			for _, source := range externalSources {
				if source.Parsed <= 0 {
					continue
				}
				advanced.Sources = append(advanced.Sources, source.Type)
			}
		}
		applyEstimatedTraffic(flows, peers, deltas)
		flowRows := c.sortedFlows(flows, options.interval, options.maxFlows)
		payload := capturePayload{
			Capture:        meta,
			Flows:          flowRows,
			SocketSnapshot: c.buildSocketSnapshot(flows, options.interval, options.maxFlows),
			Peers:          c.sortedPeers(peers, options.interval, options.maxPeers),
			Listeners:      c.sortedListeners(listeners, options.maxListeners),
			Interfaces:     deltas,
			LocalCtx:       localCtx,
		}

		c.enrichAdvancedPassive(ctx, advanced, options, localCtx, localIPs, &meta)
		if len(advanced.Sources) > 0 {
			meta.Source = strings.Join(uniqueStrings(advanced.Sources, 6), "+")
		}
		payload.Capture = meta
		if options.requestedMode != "socket" || options.pcapEnabled || len(advanced.NetlinkLinks) > 0 || advanced.PCAP != nil || len(advanced.ExternalSources) > 0 {
			payload.PassiveAdvanced = advanced
		}
		return json.Marshal(payload)
	}

	for {
		select {
		case <-ctx.Done():
			meta.Warnings = append(meta.Warnings, "coleta interrompida por contexto")
			end = time.Now().UTC()
			meta.EndedAtUnix = end.Unix()
			return finalize(ioDelta(startIO, nil))
		case <-deadline.C:
			end = time.Now().UTC()
			endIO, e := gnet.IOCountersWithContext(ctx, true)
			deltas := ioDelta(startIO, endIO)
			if e != nil {
				meta.Warnings = append(meta.Warnings, "network io counters indisponiveis no final")
			}
			meta.EndedAtUnix = end.Unix()
			return finalize(deltas)
		case <-ticker.C:
			sample()
		}
	}
}

func (c *collector) resolveOptions() passiveCollectOptions {
	options := passiveCollectOptions{
		requestedMode: "auto",
		window:        c.window,
		interval:      c.interval,
		maxFlows:      c.maxFlows,
		maxPeers:      c.maxPeers,
		maxListeners:  c.maxListeners,
		pcapEnabled:   false,
		pcapDuration:  defaultPCAPDuration,
		pcapPackets:   defaultPCAPPackets,
		externalMax:   defaultExternalMax,
	}
	if c.prefs == nil {
		options.window = clampDuration(options.window, minWindow, maxWindow, defaultWindow)
		options.interval = clampDuration(options.interval, minInterval, maxInterval, defaultInterval)
		options.maxFlows = clampInt(options.maxFlows, minMaxRows, maxMaxRows, defaultMaxFlows)
		options.maxPeers = clampInt(options.maxPeers, minMaxRows, maxMaxRows, defaultMaxPeers)
		options.maxListeners = clampInt(options.maxListeners, minMaxRows, maxMaxRows, defaultMaxListeners)
		return options
	}

	prefs := c.prefs()
	options.requestedMode = normalizePassiveMode(prefs.NetworkPassiveMode)
	if prefs.NetworkCaptureWindowSec > 0 {
		options.window = time.Duration(prefs.NetworkCaptureWindowSec) * time.Second
	}
	if prefs.NetworkCaptureSampleSec > 0 {
		options.interval = time.Duration(prefs.NetworkCaptureSampleSec) * time.Second
	}
	if prefs.NetworkCaptureMaxFlows > 0 {
		options.maxFlows = prefs.NetworkCaptureMaxFlows
	}
	if prefs.NetworkCaptureMaxPeers > 0 {
		options.maxPeers = prefs.NetworkCaptureMaxPeers
	}
	if prefs.NetworkCaptureMaxListeners > 0 {
		options.maxListeners = prefs.NetworkCaptureMaxListeners
	}
	options.pcapEnabled = prefs.NetworkPCAPEnabled
	options.pcapIface = strings.TrimSpace(prefs.NetworkPCAPIface)
	if prefs.NetworkPCAPDurationSec > 0 {
		options.pcapDuration = time.Duration(prefs.NetworkPCAPDurationSec) * time.Second
	}
	if prefs.NetworkPCAPMaxPackets > 0 {
		options.pcapPackets = prefs.NetworkPCAPMaxPackets
	}
	options.externalSpecs = append(options.externalSpecs, prefs.NetworkExternalSources...)
	if prefs.NetworkExternalMaxRecords > 0 {
		options.externalMax = prefs.NetworkExternalMaxRecords
	}

	options.window = clampDuration(options.window, minWindow, maxWindow, defaultWindow)
	options.interval = clampDuration(options.interval, minInterval, maxInterval, defaultInterval)
	options.maxFlows = clampInt(options.maxFlows, minMaxRows, maxMaxRows, defaultMaxFlows)
	options.maxPeers = clampInt(options.maxPeers, minMaxRows, maxMaxRows, defaultMaxPeers)
	options.maxListeners = clampInt(options.maxListeners, minMaxRows, maxMaxRows, defaultMaxListeners)
	options.pcapDuration = clampDuration(options.pcapDuration, time.Second, maxPCAPDuration, defaultPCAPDuration)
	options.pcapPackets = clampInt(options.pcapPackets, 16, maxPCAPPackets, defaultPCAPPackets)
	options.externalMax = clampInt(options.externalMax, 100, maxExternalMax, defaultExternalMax)

	if options.requestedMode == "pcap" {
		options.pcapEnabled = true
	}
	return options
}

func normalizePassiveMode(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", "auto":
		return "auto"
	case "socket", "socket_snapshot":
		return "socket"
	case "netlink":
		return "netlink"
	case "pcap":
		return "pcap"
	case "ebpf":
		return "ebpf"
	default:
		return "auto"
	}
}

func clampDuration(value, minValue, maxValue, fallback time.Duration) time.Duration {
	out := value
	if out <= 0 {
		out = fallback
	}
	if out < minValue {
		out = minValue
	}
	if out > maxValue {
		out = maxValue
	}
	return out
}

func clampInt(value, minValue, maxValue, fallback int) int {
	out := value
	if out <= 0 {
		out = fallback
	}
	if out < minValue {
		out = minValue
	}
	if out > maxValue {
		out = maxValue
	}
	return out
}

func detectPassiveCapabilities() passiveCapabilities {
	if runtime.GOOS != "linux" {
		return passiveCapabilities{}
	}
	ebpfAvailable := false
	if _, err := os.Stat("/sys/fs/bpf"); err == nil {
		ebpfAvailable = true
	}
	if commandExists("bpftool") {
		ebpfAvailable = true
	}
	return passiveCapabilities{
		Netlink: commandExists("ip"),
		PCAP:    commandExists("tcpdump"),
		EBPF:    ebpfAvailable,
	}
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func (c *collector) enrichAdvancedPassive(
	ctx context.Context,
	advanced *passiveAdvancedPayload,
	options passiveCollectOptions,
	localCtx *localContext,
	localIPs map[string]struct{},
	meta *captureMeta,
) {
	if advanced == nil || meta == nil {
		return
	}
	mode := normalizePassiveMode(options.requestedMode)
	if mode == "socket" {
		advanced.Sources = uniqueStrings(advanced.Sources, 8)
		advanced.AppliedMode = "socket_snapshot"
		if len(advanced.ExternalSources) > 0 {
			advanced.AppliedMode += "+external"
			meta.Mode = "passive_advanced"
		} else {
			meta.Mode = "passive"
		}
		return
	}

	runNetlink := false
	runPCAP := false
	switch mode {
	case "auto":
		runNetlink = true
		runPCAP = options.pcapEnabled
	case "netlink":
		runNetlink = true
	case "pcap":
		runPCAP = true
	case "ebpf":
		runNetlink = true
		if !advanced.Capabilities.EBPF {
			meta.Warnings = append(meta.Warnings, "ebpf indisponível no host; fallback para socket/netlink")
		} else {
			meta.Warnings = append(meta.Warnings, "ebpf detectado; usando fallback seguro com socket/netlink nesta versão")
		}
	}

	if runNetlink {
		if !advanced.Capabilities.Netlink {
			meta.Warnings = append(meta.Warnings, "netlink indisponível: comando ip não encontrado")
		} else {
			links, warningMsg := collectNetlinkLinks(ctx)
			if warningMsg != "" {
				meta.Warnings = append(meta.Warnings, warningMsg)
			}
			if len(links) > 0 {
				advanced.NetlinkLinks = links
				advanced.Sources = append(advanced.Sources, "netlink")
			}
		}
	}

	if runPCAP {
		if !advanced.Capabilities.PCAP {
			meta.Warnings = append(meta.Warnings, "pcap indisponível: tcpdump não encontrado")
		} else {
			iface := strings.TrimSpace(options.pcapIface)
			if iface == "" {
				iface = choosePCAPInterface(localCtx)
			}
			pcap, warningMsg := collectPCAPSummary(ctx, iface, options.pcapDuration, options.pcapPackets, localIPs)
			if warningMsg != "" {
				meta.Warnings = append(meta.Warnings, warningMsg)
			}
			if pcap != nil {
				advanced.PCAP = pcap
				advanced.Sources = append(advanced.Sources, "pcap")
			}
		}
	}

	advanced.Sources = uniqueStrings(advanced.Sources, 8)
	advanced.AppliedMode = strings.Join(advanced.Sources, "+")
	if advanced.AppliedMode == "" {
		advanced.AppliedMode = "socket_snapshot"
	}
	if mode == "ebpf" && advanced.Capabilities.EBPF {
		advanced.AppliedMode += "+ebpf_probe"
	}
	if len(advanced.Sources) > 1 || advanced.PCAP != nil || len(advanced.NetlinkLinks) > 0 || len(advanced.ExternalSources) > 0 || mode == "ebpf" {
		meta.Mode = "passive_advanced"
	}
}

func collectNetlinkLinks(ctx context.Context) ([]netlinkLinkRow, string) {
	if runtime.GOOS != "linux" {
		return nil, ""
	}
	raw, err := runCommand(ctx, 1500*time.Millisecond, "ip", "-j", "-s", "link", "show")
	if err != nil {
		return nil, "netlink indisponível: falha ao executar ip -j -s link"
	}
	rows := parseNetlinkLinksJSON(raw)
	if len(rows) == 0 {
		return nil, "netlink indisponível: saída vazia ou inválida"
	}
	return rows, ""
}

type netlinkJSONStats struct {
	Bytes   uint64 `json:"bytes"`
	Packets uint64 `json:"packets"`
	Errors  uint64 `json:"errors"`
	Dropped uint64 `json:"dropped"`
}

type netlinkJSONStatsPair struct {
	RX netlinkJSONStats `json:"rx"`
	TX netlinkJSONStats `json:"tx"`
}

type netlinkJSONLink struct {
	IfName    string                `json:"ifname"`
	OperState string                `json:"operstate"`
	Address   string                `json:"address"`
	MTU       int                   `json:"mtu"`
	Stats64   *netlinkJSONStatsPair `json:"stats64"`
	Stats     *netlinkJSONStatsPair `json:"stats"`
}

func parseNetlinkLinksJSON(raw []byte) []netlinkLinkRow {
	var items []netlinkJSONLink
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	out := make([]netlinkLinkRow, 0, len(items))
	for _, item := range items {
		ifName := strings.TrimSpace(item.IfName)
		if ifName == "" {
			continue
		}
		stats := item.Stats64
		if stats == nil {
			stats = item.Stats
		}
		row := netlinkLinkRow{
			Name:         ifName,
			OperState:    strings.ToLower(strings.TrimSpace(item.OperState)),
			HardwareAddr: strings.TrimSpace(item.Address),
			MTU:          item.MTU,
		}
		if stats != nil {
			row.RXBytes = stats.RX.Bytes
			row.RXPackets = stats.RX.Packets
			row.RXErrors = stats.RX.Errors
			row.RXDrops = stats.RX.Dropped
			row.TXBytes = stats.TX.Bytes
			row.TXPackets = stats.TX.Packets
			row.TXErrors = stats.TX.Errors
			row.TXDrops = stats.TX.Dropped
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if len(out) > 128 {
		out = out[:128]
	}
	return out
}

func choosePCAPInterface(localCtx *localContext) string {
	if localCtx != nil {
		for _, route := range localCtx.Routes {
			if route.IsDefault && strings.TrimSpace(route.Interface) != "" {
				return strings.TrimSpace(route.Interface)
			}
		}
		for _, iface := range localCtx.Interfaces {
			name := strings.TrimSpace(iface.Name)
			if name == "" || strings.EqualFold(name, "lo") {
				continue
			}
			if iface.IsUp {
				return name
			}
		}
	}
	return "any"
}

func collectPCAPSummary(
	ctx context.Context,
	iface string,
	duration time.Duration,
	maxPackets int,
	localIPs map[string]struct{},
) (*pcapSummary, string) {
	if runtime.GOOS != "linux" {
		return nil, ""
	}
	iface = strings.TrimSpace(iface)
	if iface == "" {
		iface = "any"
	}
	duration = clampDuration(duration, time.Second, maxPCAPDuration, defaultPCAPDuration)
	maxPackets = clampInt(maxPackets, 16, maxPCAPPackets, defaultPCAPPackets)
	raw, err := runCommand(ctx, duration+1500*time.Millisecond, "tcpdump", "-i", iface, "-nn", "-q", "-l", "-c", strconv.Itoa(maxPackets))
	summary := parseTCPDumpOutput(string(raw), localIPs)
	summary.Interface = iface
	summary.RequestedDuration = int(duration.Seconds())
	summary.RequestedPackets = maxPackets
	if err != nil && summary.CapturedPackets == 0 && len(summary.Flows) == 0 {
		reason := strings.ToLower(string(raw))
		switch {
		case strings.Contains(reason, "permission denied"), strings.Contains(reason, "you don't have permission"):
			return &summary, "pcap indisponível: permissão insuficiente para tcpdump"
		case strings.Contains(reason, "no such device"):
			return &summary, "pcap indisponível: interface de captura inválida"
		default:
			return &summary, "pcap controlado sem pacotes no período"
		}
	}
	return &summary, ""
}

func runCommand(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(timeoutCtx, name, args...)
	return cmd.CombinedOutput()
}

type pcapFlowKey struct {
	proto      string
	direction  string
	localIP    string
	localPort  uint32
	remoteIP   string
	remotePort uint32
}

func parseTCPDumpOutput(raw string, localIPs map[string]struct{}) pcapSummary {
	lines := strings.Split(raw, "\n")
	flows := make(map[pcapFlowKey]int)
	out := pcapSummary{}
	for _, line := range lines {
		clean := strings.TrimSpace(line)
		if clean == "" {
			continue
		}
		out.CapturedPackets = parseTCPDumpCounter(clean, "packets captured", out.CapturedPackets)
		out.ReceivedByFilter = parseTCPDumpCounter(clean, "packets received by filter", out.ReceivedByFilter)
		out.DroppedByKernel = parseTCPDumpCounter(clean, "packets dropped by kernel", out.DroppedByKernel)

		matches := tcpdumpLineRegex.FindStringSubmatch(clean)
		if len(matches) != 5 {
			continue
		}
		srcIP, srcPort := parseTCPDumpEndpoint(matches[2])
		dstIP, dstPort := parseTCPDumpEndpoint(matches[3])
		if srcIP == "" || dstIP == "" {
			continue
		}
		direction, localIP, localPort, remoteIP, remotePort := inferPCAPDirection(srcIP, srcPort, dstIP, dstPort, localIPs)
		key := pcapFlowKey{
			proto:      inferPCAPProtocol(matches[4], srcPort, dstPort),
			direction:  direction,
			localIP:    localIP,
			localPort:  localPort,
			remoteIP:   remoteIP,
			remotePort: remotePort,
		}
		flows[key]++
	}

	out.Flows = make([]pcapFlowRow, 0, len(flows))
	for key, samples := range flows {
		out.Flows = append(out.Flows, pcapFlowRow{
			Protocol:   key.proto,
			Direction:  key.direction,
			LocalIP:    key.localIP,
			LocalPort:  key.localPort,
			RemoteIP:   key.remoteIP,
			RemotePort: key.remotePort,
			Samples:    samples,
		})
	}
	sort.Slice(out.Flows, func(i, j int) bool {
		if out.Flows[i].Samples == out.Flows[j].Samples {
			if out.Flows[i].RemoteIP == out.Flows[j].RemoteIP {
				return out.Flows[i].RemotePort < out.Flows[j].RemotePort
			}
			return out.Flows[i].RemoteIP < out.Flows[j].RemoteIP
		}
		return out.Flows[i].Samples > out.Flows[j].Samples
	})
	if len(out.Flows) > 200 {
		out.Flows = out.Flows[:200]
	}
	return out
}

func parseTCPDumpCounter(line, suffix string, fallback int) int {
	if !strings.HasSuffix(line, suffix) {
		return fallback
	}
	idx := strings.Index(line, " ")
	if idx <= 0 {
		return fallback
	}
	value, err := strconv.Atoi(strings.TrimSpace(line[:idx]))
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

func parseTCPDumpEndpoint(raw string) (string, uint32) {
	value := strings.TrimSpace(strings.TrimSuffix(raw, ","))
	value = strings.Trim(value, "[]")
	if value == "" {
		return "", 0
	}
	if idx := strings.LastIndex(value, "."); idx > 0 {
		portValue := strings.TrimSpace(value[idx+1:])
		port, err := strconv.Atoi(portValue)
		if err == nil && port >= 0 && port <= 65535 {
			host := strings.TrimSpace(value[:idx])
			if host != "" {
				return normalizeEndpointHost(host), uint32(port)
			}
		}
	}
	return normalizeEndpointHost(value), 0
}

func normalizeEndpointHost(raw string) string {
	host := strings.TrimSpace(raw)
	if i := strings.Index(host, "%"); i > 0 {
		host = host[:i]
	}
	host = strings.Trim(host, "[]")
	return host
}

func inferPCAPDirection(
	srcIP string,
	srcPort uint32,
	dstIP string,
	dstPort uint32,
	localIPs map[string]struct{},
) (direction, localIP string, localPort uint32, remoteIP string, remotePort uint32) {
	srcLocal := isLocalPeerIP(srcIP, localIPs)
	dstLocal := isLocalPeerIP(dstIP, localIPs)
	switch {
	case srcLocal && !dstLocal:
		return "egress", srcIP, srcPort, dstIP, dstPort
	case dstLocal && !srcLocal:
		return "ingress", dstIP, dstPort, srcIP, srcPort
	case srcLocal && dstLocal:
		return "lateral", srcIP, srcPort, dstIP, dstPort
	default:
		return "unknown", srcIP, srcPort, dstIP, dstPort
	}
}

func isLocalPeerIP(ipRaw string, localIPs map[string]struct{}) bool {
	value := strings.TrimSpace(ipRaw)
	if value == "" {
		return false
	}
	if _, ok := localIPs[value]; ok {
		return true
	}
	parsed := net.ParseIP(value)
	return parsed != nil && parsed.IsLoopback()
}

func inferPCAPProtocol(payload string, srcPort, dstPort uint32) string {
	upper := strings.ToUpper(strings.TrimSpace(payload))
	if strings.Contains(upper, "UDP") {
		return "udp"
	}
	if strings.Contains(upper, "ICMP") {
		return "ip"
	}
	if strings.Contains(upper, "FLAGS") || strings.Contains(upper, "SEQ") || strings.Contains(upper, "ACK") {
		return "tcp"
	}
	if srcPort == 53 || dstPort == 53 {
		return "udp"
	}
	if srcPort > 0 || dstPort > 0 {
		return "tcp"
	}
	return "ip"
}

var externalSourceTypes = map[string]struct{}{
	"firewall": {},
	"ndr":      {},
	"siem":     {},
	"netflow":  {},
	"sflow":    {},
	"ipfix":    {},
}

func mergeExternalSources(
	options passiveCollectOptions,
	localIPs map[string]struct{},
	flows map[flowKey]*flowAgg,
	peers map[peerKey]*peerAgg,
	meta *captureMeta,
) []externalSourceStatus {
	if len(options.externalSpecs) == 0 {
		return nil
	}
	statuses := make([]externalSourceStatus, 0, len(options.externalSpecs))
	for _, rawSpec := range options.externalSpecs {
		specType, specPath := parseExternalSourceSpec(rawSpec)
		status := externalSourceStatus{Type: specType, Path: specPath}
		if specPath == "" {
			status.LastError = "fonte externa inválida"
			statuses = append(statuses, status)
			continue
		}
		observations, dropped, err := readExternalSource(specType, specPath, options.externalMax)
		status.Parsed = len(observations)
		status.Dropped = dropped
		if err != nil {
			status.LastError = err.Error()
			meta.Warnings = append(meta.Warnings, "fonte externa indisponível: "+specType+" "+specPath)
		}
		for _, obs := range observations {
			mergeExternalObservation(obs, localIPs, flows, peers)
		}
		statuses = append(statuses, status)
	}
	return statuses
}

func parseExternalSourceSpec(raw string) (sourceType, path string) {
	spec := strings.TrimSpace(raw)
	if spec == "" {
		return "siem", ""
	}
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) == 2 {
		rawType := strings.ToLower(strings.TrimSpace(parts[0]))
		if _, known := externalSourceTypes[rawType]; known {
			p := strings.TrimSpace(parts[1])
			if p != "" {
				return rawType, p
			}
		}
	}
	detectedType := normalizeExternalTypeFromPath(spec)
	return detectedType, spec
}

func normalizeExternalSourceType(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" || value == "auto" {
		return "siem"
	}
	if _, ok := externalSourceTypes[value]; ok {
		return value
	}
	return "siem"
}

func normalizeExternalTypeFromPath(path string) string {
	name := strings.ToLower(strings.TrimSpace(filepath.Base(path)))
	switch {
	case strings.Contains(name, "netflow"):
		return "netflow"
	case strings.Contains(name, "sflow"):
		return "sflow"
	case strings.Contains(name, "ipfix"):
		return "ipfix"
	case strings.Contains(name, "firewall"), strings.Contains(name, "fw"):
		return "firewall"
	case strings.Contains(name, "ndr"):
		return "ndr"
	default:
		return "siem"
	}
}

func readExternalSource(sourceType, path string, maxRows int) ([]externalObservation, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buffer := make([]byte, 0, 1024*64)
	scanner.Buffer(buffer, 1024*1024)
	observations := make([]externalObservation, 0, minInt(maxRows, 256))
	dropped := 0
	for scanner.Scan() {
		if len(observations) >= maxRows {
			dropped++
			continue
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		obs, ok := parseExternalObservationLine(sourceType, line)
		if !ok {
			dropped++
			continue
		}
		observations = append(observations, obs)
	}
	if err := scanner.Err(); err != nil {
		return observations, dropped, err
	}
	return observations, dropped, nil
}

func parseExternalObservationLine(sourceType, line string) (externalObservation, bool) {
	var payload map[string]any
	if strings.HasPrefix(strings.TrimSpace(line), "{") {
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			return externalObservation{}, false
		}
		payload = normalizeAnyMapKeys(payload)
	} else {
		payload = parseExternalKVLine(line)
	}
	if len(payload) == 0 {
		return externalObservation{}, false
	}
	obs := externalObservation{
		sourceType: normalizeExternalSourceType(sourceType),
		protocol:   normalizeProtocol(getStringAny(payload, "protocol", "proto", "l4_protocol", "ip_protocol")),
		direction:  normalizeDirection(getStringAny(payload, "direction", "flow_direction", "traffic_direction")),
		state:      strings.ToUpper(strings.TrimSpace(getStringAny(payload, "state", "conn_state", "tcp_state"))),
		srcIP:      strings.TrimSpace(getStringAny(payload, "src_ip", "source_ip", "src_addr", "ipv4_src_addr", "ipv6_src_addr", "client_ip")),
		dstIP:      strings.TrimSpace(getStringAny(payload, "dst_ip", "destination_ip", "dst_addr", "ipv4_dst_addr", "ipv6_dst_addr", "remote_ip", "server_ip")),
		srcPort:    uint32(getIntAny(payload, "src_port", "source_port", "l4_src_port", "client_port")),
		dstPort:    uint32(getIntAny(payload, "dst_port", "destination_port", "l4_dst_port", "remote_port", "server_port")),
		bytes:      getIntAny(payload, "bytes", "octets", "byte_count", "in_bytes", "out_bytes"),
		packets:    getIntAny(payload, "packets", "pkts", "packet_count", "in_pkts", "out_pkts"),
		firstSeen:  getTimeAny(payload, "first_seen", "start_time", "flow_start", "timestamp", "@timestamp", "time"),
		lastSeen:   getTimeAny(payload, "last_seen", "end_time", "flow_end", "timestamp", "@timestamp", "time"),
		dnsQuery:   normalizeResolutionHost(getStringAny(payload, "dns_query", "query", "domain", "host")),
		dnsAnswer:  strings.TrimSpace(getStringAny(payload, "dns_answer", "resolved_ip", "answer")),
		sni:        normalizeResolutionHost(getStringAny(payload, "sni", "tls_sni")),
		tlsSubject: normalizeResolutionHost(getStringAny(payload, "tls_subject", "subject", "x509_subject")),
		resetHits:  getIntAny(payload, "resets", "tcp_resets", "rst_count"),
		timeoutHits: getIntAny(payload,
			"timeouts", "timeout_count"),
	}
	if obs.firstSeen == 0 {
		obs.firstSeen = time.Now().UTC().Unix()
	}
	if obs.lastSeen == 0 || obs.lastSeen < obs.firstSeen {
		obs.lastSeen = obs.firstSeen
	}
	if obs.srcIP == "" || obs.dstIP == "" {
		return externalObservation{}, false
	}
	return obs, true
}

func normalizeAnyMapKeys(raw map[string]any) map[string]any {
	out := make(map[string]any, len(raw))
	for key, value := range raw {
		out[strings.ToLower(strings.TrimSpace(key))] = value
	}
	return out
}

func parseExternalKVLine(line string) map[string]any {
	fields := strings.Fields(line)
	out := make(map[string]any, len(fields))
	for _, token := range fields {
		if !strings.Contains(token, "=") {
			continue
		}
		kv := strings.SplitN(token, "=", 2)
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		value := strings.Trim(strings.TrimSpace(kv[1]), `"'`)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func getStringAny(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		switch value := raw.(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		case float64:
			return strconv.FormatFloat(value, 'f', -1, 64)
		case int:
			return strconv.Itoa(value)
		case json.Number:
			return value.String()
		}
	}
	return ""
}

func getIntAny(payload map[string]any, keys ...string) int {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		switch value := raw.(type) {
		case float64:
			return int(value)
		case float32:
			return int(value)
		case int:
			return value
		case int64:
			return int(value)
		case uint64:
			return int(value)
		case string:
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err == nil {
				return n
			}
		case json.Number:
			n, err := value.Int64()
			if err == nil {
				return int(n)
			}
		}
	}
	return 0
}

func getTimeAny(payload map[string]any, keys ...string) int64 {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		switch value := raw.(type) {
		case float64:
			return normalizeUnixTime(int64(value))
		case int:
			return normalizeUnixTime(int64(value))
		case int64:
			return normalizeUnixTime(value)
		case string:
			trimmed := strings.TrimSpace(value)
			if trimmed == "" {
				continue
			}
			if n, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
				return normalizeUnixTime(n)
			}
			if ts, err := time.Parse(time.RFC3339, trimmed); err == nil {
				return ts.UTC().Unix()
			}
		case json.Number:
			if n, err := value.Int64(); err == nil {
				return normalizeUnixTime(n)
			}
		}
	}
	return 0
}

func normalizeUnixTime(value int64) int64 {
	if value <= 0 {
		return 0
	}
	if value > 1_000_000_000_000 {
		return value / 1000
	}
	return value
}

func normalizeProtocol(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "6", "tcp":
		return "tcp"
	case "17", "udp":
		return "udp"
	case "1", "58", "icmp", "icmpv6":
		return "ip"
	case "":
		return "ip"
	default:
		return value
	}
}

func normalizeDirection(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "inbound", "ingress", "entrada":
		return "ingress"
	case "outbound", "egress", "saida", "saída":
		return "egress"
	case "lateral":
		return "lateral"
	default:
		return ""
	}
}

func mergeExternalObservation(
	obs externalObservation,
	localIPs map[string]struct{},
	flows map[flowKey]*flowAgg,
	peers map[peerKey]*peerAgg,
) {
	direction := obs.direction
	localIP := obs.srcIP
	localPort := obs.srcPort
	remoteIP := obs.dstIP
	remotePort := obs.dstPort
	if direction == "" {
		direction, localIP, localPort, remoteIP, remotePort = inferPCAPDirection(obs.srcIP, obs.srcPort, obs.dstIP, obs.dstPort, localIPs)
	}
	scope := remoteScope(remoteIP, localIPs)
	reverseDNS := normalizeResolutionHost(obs.dnsQuery)
	if reverseDNS == "" {
		reverseDNS = normalizeResolutionHost(obs.sni)
	}
	baseRisk := classifyNetworkRisk(remotePort, scope, direction, obs.protocol)
	failureRisk := classifyNetworkFailureRisk(
		remotePort,
		scope,
		direction,
		obs.protocol,
		obs.state,
		obs.dnsQuery,
		obs.dnsAnswer,
		1,
		obs.resetHits,
		obs.timeoutHits,
	)
	key := flowKey{
		proto:       normalizeProtocol(obs.protocol),
		direction:   direction,
		localIP:     localIP,
		localPort:   localPort,
		remoteIP:    remoteIP,
		remotePort:  remotePort,
		pid:         0,
		parentPID:   0,
		process:     "ext_" + obs.sourceType,
		serviceName: obs.sourceType,
	}
	flow, ok := flows[key]
	if !ok {
		flow = &flowAgg{
			reasons: make(map[string]struct{}),
			tags:    make(map[string]struct{}),
		}
		flows[key] = flow
	}
	flow.samples++
	if flow.firstSeen == 0 || obs.firstSeen < flow.firstSeen {
		flow.firstSeen = obs.firstSeen
	}
	if obs.lastSeen > flow.lastSeen {
		flow.lastSeen = obs.lastSeen
	}
	flow.state = obs.state
	flow.scope = scope
	if flow.reverseDNS == "" {
		flow.reverseDNS = reverseDNS
	}
	if flow.dnsQuery == "" {
		flow.dnsQuery = obs.dnsQuery
	}
	if flow.dnsAnswer == "" {
		flow.dnsAnswer = obs.dnsAnswer
	}
	if flow.sni == "" {
		flow.sni = obs.sni
	}
	if flow.tlsSubject == "" {
		flow.tlsSubject = obs.tlsSubject
	}
	if obs.bytes > 0 {
		if direction == "ingress" {
			flow.bytesIn += obs.bytes
		} else if direction == "egress" {
			flow.bytesOut += obs.bytes
		} else {
			half := obs.bytes / 2
			flow.bytesIn += half
			flow.bytesOut += obs.bytes - half
		}
	}
	if obs.packets > 0 {
		if direction == "ingress" {
			flow.packetsIn += obs.packets
		} else if direction == "egress" {
			flow.packetsOut += obs.packets
		} else {
			half := obs.packets / 2
			flow.packetsIn += half
			flow.packetsOut += obs.packets - half
		}
	}
	deltaBytesIn := 0
	deltaBytesOut := 0
	deltaPacketsIn := 0
	deltaPacketsOut := 0
	if obs.bytes > 0 {
		if direction == "ingress" {
			deltaBytesIn = obs.bytes
		} else if direction == "egress" {
			deltaBytesOut = obs.bytes
		} else {
			deltaBytesIn = obs.bytes / 2
			deltaBytesOut = obs.bytes - deltaBytesIn
		}
	}
	if obs.packets > 0 {
		if direction == "ingress" {
			deltaPacketsIn = obs.packets
		} else if direction == "egress" {
			deltaPacketsOut = obs.packets
		} else {
			deltaPacketsIn = obs.packets / 2
			deltaPacketsOut = obs.packets - deltaPacketsIn
		}
	}
	flow.resetHits += maxInt(obs.resetHits, 0)
	flow.timeoutHits += maxInt(obs.timeoutHits, 0)
	mergeNetworkRisk(&flow.problematic, flow.reasons, flow.tags, baseRisk)
	mergeNetworkRisk(&flow.problematic, flow.reasons, flow.tags, failureRisk)

	pk := peerKey{
		proto:      key.proto,
		direction:  direction,
		remoteIP:   remoteIP,
		remotePort: remotePort,
		scope:      scope,
	}
	peer, ok := peers[pk]
	if !ok {
		peer = &peerAgg{
			processes:    make(map[string]struct{}),
			serviceNames: make(map[string]struct{}),
			processUsers: make(map[string]struct{}),
			reasons:      make(map[string]struct{}),
			tags:         make(map[string]struct{}),
		}
		peers[pk] = peer
	}
	peer.samples++
	if peer.firstSeen == 0 || obs.firstSeen < peer.firstSeen {
		peer.firstSeen = obs.firstSeen
	}
	if obs.lastSeen > peer.lastSeen {
		peer.lastSeen = obs.lastSeen
	}
	if peer.reverseDNS == "" {
		peer.reverseDNS = reverseDNS
	}
	if peer.dnsQuery == "" {
		peer.dnsQuery = obs.dnsQuery
	}
	if peer.dnsAnswer == "" {
		peer.dnsAnswer = obs.dnsAnswer
	}
	if peer.sni == "" {
		peer.sni = obs.sni
	}
	if peer.tlsSubject == "" {
		peer.tlsSubject = obs.tlsSubject
	}
	peer.bytesIn += deltaBytesIn
	peer.bytesOut += deltaBytesOut
	peer.packetsIn += deltaPacketsIn
	peer.packetsOut += deltaPacketsOut
	peer.resetHits += maxInt(obs.resetHits, 0)
	peer.timeoutHits += maxInt(obs.timeoutHits, 0)
	peer.processes["ext_"+obs.sourceType] = struct{}{}
	peer.serviceNames[obs.sourceType] = struct{}{}
	mergeNetworkRisk(&peer.problematic, peer.reasons, peer.tags, baseRisk)
	mergeNetworkRisk(&peer.problematic, peer.reasons, peer.tags, failureRisk)
}

func minInt(a, b int) int {
	if a <= b {
		return a
	}
	return b
}

func (c *collector) sortedFlows(flows map[flowKey]*flowAgg, sampleInterval time.Duration, maxFlows int) []flowRow {
	out := make([]flowRow, 0, len(flows))
	for k, v := range flows {
		out = append(out, flowRow{
			Protocol:          k.proto,
			Direction:         k.direction,
			LocalIP:           k.localIP,
			LocalPort:         k.localPort,
			RemoteIP:          k.remoteIP,
			ReverseDNS:        v.reverseDNS,
			DNSQuery:          v.dnsQuery,
			DNSAnswer:         v.dnsAnswer,
			SNI:               v.sni,
			TLSSubject:        v.tlsSubject,
			FirstSeenUnix:     v.firstSeen,
			LastSeenUnix:      v.lastSeen,
			RemotePort:        k.remotePort,
			RemoteScope:       v.scope,
			State:             v.state,
			PID:               k.pid,
			ParentPID:         k.parentPID,
			Process:           k.process,
			ServiceName:       k.serviceName,
			ProcessUser:       k.processUser,
			ProcessExe:        k.processExe,
			ProcessCmd:        k.processCmd,
			BytesIn:           v.bytesIn,
			BytesOut:          v.bytesOut,
			PacketsIn:         v.packetsIn,
			PacketsOut:        v.packetsOut,
			ResetHits:         v.resetHits,
			TimeoutHits:       v.timeoutHits,
			AvgDurationSec:    estimateAvgDurationSec(v.firstSeen, v.lastSeen, sampleInterval),
			IsProblematic:     v.problematic,
			ProblematicReason: joinSortedSet(v.reasons, " | "),
			RiskTags:          sortedSetValues(v.tags, 6),
			Samples:           v.samples,
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
	if len(out) > maxFlows {
		out = out[:maxFlows]
	}
	return out
}

func (c *collector) buildSocketSnapshot(flows map[flowKey]*flowAgg, sampleInterval time.Duration, maxFlows int) []socketSnapshotRow {
	merged := make(map[socketSnapshotKey]*socketSnapshotAgg)
	for k, v := range flows {
		sk := socketSnapshotKey{
			proto:      k.proto,
			direction:  k.direction,
			localIP:    k.localIP,
			localPort:  k.localPort,
			remoteIP:   k.remoteIP,
			remotePort: k.remotePort,
			state:      v.state,
		}
		agg, ok := merged[sk]
		if !ok {
			agg = &socketSnapshotAgg{}
			merged[sk] = agg
		}
		agg.samples += v.samples
		if v.firstSeen > 0 && (agg.firstSeen == 0 || v.firstSeen < agg.firstSeen) {
			agg.firstSeen = v.firstSeen
		}
		if v.lastSeen > agg.lastSeen {
			agg.lastSeen = v.lastSeen
		}
		if agg.dnsQuery == "" && v.dnsQuery != "" {
			agg.dnsQuery = v.dnsQuery
		}
		if agg.dnsAnswer == "" && v.dnsAnswer != "" {
			agg.dnsAnswer = v.dnsAnswer
		}
		if agg.sni == "" && v.sni != "" {
			agg.sni = v.sni
		}
		if agg.tlsSubject == "" && v.tlsSubject != "" {
			agg.tlsSubject = v.tlsSubject
		}
		agg.resetHits += v.resetHits
		agg.timeoutHits += v.timeoutHits
		agg.bytesIn += v.bytesIn
		agg.bytesOut += v.bytesOut
		agg.packetsIn += v.packetsIn
		agg.packetsOut += v.packetsOut
		avgDurationSec := estimateAvgDurationSec(v.firstSeen, v.lastSeen, sampleInterval)
		if avgDurationSec > 0 {
			weight := float64(maxInt(v.samples, 1))
			agg.durationWeighted += avgDurationSec * weight
			agg.durationWeight += weight
		}
	}

	out := make([]socketSnapshotRow, 0, len(merged))
	for k, v := range merged {
		out = append(out, socketSnapshotRow{
			Protocol:       k.proto,
			Direction:      k.direction,
			LocalIP:        k.localIP,
			LocalPort:      k.localPort,
			RemoteIP:       k.remoteIP,
			RemotePort:     k.remotePort,
			DNSQuery:       v.dnsQuery,
			DNSAnswer:      v.dnsAnswer,
			SNI:            v.sni,
			TLSSubject:     v.tlsSubject,
			State:          k.state,
			BytesIn:        v.bytesIn,
			BytesOut:       v.bytesOut,
			PacketsIn:      v.packetsIn,
			PacketsOut:     v.packetsOut,
			ResetHits:      v.resetHits,
			TimeoutHits:    v.timeoutHits,
			AvgDurationSec: averageDuration(v.durationWeighted, v.durationWeight),
			FirstSeenUnix:  v.firstSeen,
			LastSeenUnix:   v.lastSeen,
			Samples:        v.samples,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Samples == out[j].Samples {
			if out[i].RemoteIP == out[j].RemoteIP {
				if out[i].RemotePort == out[j].RemotePort {
					if out[i].LocalIP == out[j].LocalIP {
						return out[i].LocalPort < out[j].LocalPort
					}
					return out[i].LocalIP < out[j].LocalIP
				}
				return out[i].RemotePort < out[j].RemotePort
			}
			return out[i].RemoteIP < out[j].RemoteIP
		}
		return out[i].Samples > out[j].Samples
	})
	if len(out) > maxFlows {
		out = out[:maxFlows]
	}
	return out
}

func (c *collector) sortedPeers(peers map[peerKey]*peerAgg, sampleInterval time.Duration, maxPeers int) []peerRow {
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

		serviceNames := make([]string, 0, len(v.serviceNames))
		for serviceName := range v.serviceNames {
			serviceNames = append(serviceNames, serviceName)
		}
		sort.Strings(serviceNames)
		if len(serviceNames) > 5 {
			serviceNames = serviceNames[:5]
		}

		processUsers := make([]string, 0, len(v.processUsers))
		for username := range v.processUsers {
			processUsers = append(processUsers, username)
		}
		sort.Strings(processUsers)
		if len(processUsers) > 5 {
			processUsers = processUsers[:5]
		}

		out = append(out, peerRow{
			Protocol:          k.proto,
			Direction:         k.direction,
			RemoteIP:          k.remoteIP,
			ReverseDNS:        v.reverseDNS,
			DNSQuery:          v.dnsQuery,
			DNSAnswer:         v.dnsAnswer,
			SNI:               v.sni,
			TLSSubject:        v.tlsSubject,
			FirstSeenUnix:     v.firstSeen,
			LastSeenUnix:      v.lastSeen,
			RemotePort:        k.remotePort,
			RemoteScope:       k.scope,
			Samples:           v.samples,
			Processes:         processes,
			ServiceNames:      serviceNames,
			ProcessUsers:      processUsers,
			BytesIn:           v.bytesIn,
			BytesOut:          v.bytesOut,
			PacketsIn:         v.packetsIn,
			PacketsOut:        v.packetsOut,
			ResetHits:         v.resetHits,
			TimeoutHits:       v.timeoutHits,
			AvgDurationSec:    estimateAvgDurationSec(v.firstSeen, v.lastSeen, sampleInterval),
			IsProblematic:     v.problematic,
			ProblematicReason: joinSortedSet(v.reasons, " | "),
			RiskTags:          sortedSetValues(v.tags, 6),
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
	if len(out) > maxPeers {
		out = out[:maxPeers]
	}
	return out
}

func (c *collector) sortedListeners(listeners map[listenerKey]*listenerAgg, maxListeners int) []listenerRow {
	out := make([]listenerRow, 0, len(listeners))
	for k, v := range listeners {
		out = append(out, listenerRow{
			Protocol:    k.proto,
			LocalIP:     k.localIP,
			LocalPort:   k.localPort,
			State:       k.state,
			PID:         k.pid,
			ParentPID:   k.parentPID,
			Process:     k.process,
			ServiceName: k.serviceName,
			ProcessUser: k.processUser,
			ProcessExe:  k.processExe,
			ProcessCmd:  k.processCmd,
			Samples:     v.samples,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Samples == out[j].Samples {
			if out[i].LocalPort == out[j].LocalPort {
				return out[i].Protocol < out[j].Protocol
			}
			return out[i].LocalPort < out[j].LocalPort
		}
		return out[i].Samples > out[j].Samples
	})
	if len(out) > maxListeners {
		out = out[:maxListeners]
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

func applyEstimatedTraffic(flows map[flowKey]*flowAgg, peers map[peerKey]*peerAgg, deltas []ifaceDelta) {
	if len(flows) == 0 || len(deltas) == 0 {
		return
	}
	var totalBytesSent uint64
	var totalBytesRecv uint64
	var totalPacketsSent uint64
	var totalPacketsRecv uint64
	for _, delta := range deltas {
		totalBytesSent += delta.BytesSent
		totalBytesRecv += delta.BytesRecv
		totalPacketsSent += delta.PacketsSent
		totalPacketsRecv += delta.PacketsRecv
	}
	totalSamples := 0
	for _, flow := range flows {
		if flow.bytesIn > 0 || flow.bytesOut > 0 || flow.packetsIn > 0 || flow.packetsOut > 0 {
			continue
		}
		totalSamples += maxInt(flow.samples, 1)
	}
	if totalSamples <= 0 {
		return
	}

	for key, flow := range flows {
		if flow.bytesIn > 0 || flow.bytesOut > 0 || flow.packetsIn > 0 || flow.packetsOut > 0 {
			continue
		}
		weight := float64(maxInt(flow.samples, 1)) / float64(totalSamples)
		direction := strings.ToLower(strings.TrimSpace(key.direction))
		var bytesIn uint64
		var bytesOut uint64
		var packetsIn uint64
		var packetsOut uint64
		switch direction {
		case "egress":
			bytesOut = scaleUint64(totalBytesSent, weight)
			packetsOut = scaleUint64(totalPacketsSent, weight)
			bytesIn = scaleUint64(totalBytesRecv, weight*0.35)
			packetsIn = scaleUint64(totalPacketsRecv, weight*0.35)
		case "ingress":
			bytesIn = scaleUint64(totalBytesRecv, weight)
			packetsIn = scaleUint64(totalPacketsRecv, weight)
			bytesOut = scaleUint64(totalBytesSent, weight*0.35)
			packetsOut = scaleUint64(totalPacketsSent, weight*0.35)
		default:
			bytesIn = scaleUint64(totalBytesRecv, weight*0.5)
			packetsIn = scaleUint64(totalPacketsRecv, weight*0.5)
			bytesOut = scaleUint64(totalBytesSent, weight*0.5)
			packetsOut = scaleUint64(totalPacketsSent, weight*0.5)
		}

		flow.bytesIn = clampUint64ToInt(bytesIn)
		flow.bytesOut = clampUint64ToInt(bytesOut)
		flow.packetsIn = clampUint64ToInt(packetsIn)
		flow.packetsOut = clampUint64ToInt(packetsOut)

		peerKeyRef := peerKey{
			proto:      key.proto,
			direction:  key.direction,
			remoteIP:   key.remoteIP,
			remotePort: key.remotePort,
			scope:      flow.scope,
		}
		if peer, ok := peers[peerKeyRef]; ok {
			peer.bytesIn += flow.bytesIn
			peer.bytesOut += flow.bytesOut
			peer.packetsIn += flow.packetsIn
			peer.packetsOut += flow.packetsOut
		}
	}
}

func scaleUint64(total uint64, weight float64) uint64 {
	if total == 0 || weight <= 0 {
		return 0
	}
	return uint64(float64(total) * weight)
}

func clampUint64ToInt(value uint64) int {
	maxIntValue := uint64(^uint(0) >> 1)
	if value > maxIntValue {
		return int(maxIntValue)
	}
	return int(value)
}

func collectLocalContext(ctx context.Context, hostname string) (*localContext, []string) {
	out := &localContext{
		CapturedAtUnix: time.Now().UTC().Unix(),
		Hostname:       strings.TrimSpace(hostname),
	}
	warnings := make([]string, 0, 3)

	out.Interfaces = collectLocalIfaces(ctx)

	routes, routeWarning := collectLocalRoutes()
	if routeWarning != "" {
		warnings = append(warnings, routeWarning)
	}
	out.Routes = routes
	for _, route := range routes {
		if route.IsDefault && route.Gateway != "" {
			out.DefaultGateway = route.Gateway
			break
		}
	}

	dnsServers, dnsErr := readResolvConfDNSServers("/etc/resolv.conf")
	if dnsErr != nil && runtime.GOOS == "linux" {
		warnings = append(warnings, "dns local indisponivel")
	}
	out.DNSServers = dnsServers

	neighbors, neighWarnings := collectLocalNeighbors()
	out.Neighbors = neighbors
	warnings = append(warnings, neighWarnings...)

	return out, warnings
}

func collectLocalIfaces(ctx context.Context) []localIfaceRow {
	ifaces, err := gnet.InterfacesWithContext(ctx)
	if err != nil {
		return nil
	}
	out := make([]localIfaceRow, 0, len(ifaces))
	for _, inf := range ifaces {
		row := localIfaceRow{
			Name:         strings.TrimSpace(inf.Name),
			Mtu:          int(inf.MTU),
			HardwareAddr: strings.TrimSpace(inf.HardwareAddr),
			Flags:        append([]string(nil), inf.Flags...),
		}
		sort.Strings(row.Flags)
		for _, flag := range row.Flags {
			if strings.EqualFold(strings.TrimSpace(flag), "up") {
				row.IsUp = true
				break
			}
		}
		addrs := make([]string, 0, len(inf.Addrs))
		for _, addr := range inf.Addrs {
			value := strings.TrimSpace(addr.Addr)
			if value == "" {
				continue
			}
			addrs = append(addrs, value)
		}
		sort.Strings(addrs)
		if len(addrs) > 12 {
			addrs = addrs[:12]
		}
		row.Addrs = addrs
		if row.Name == "" {
			continue
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

func collectLocalRoutes() ([]localRouteRow, string) {
	if runtime.GOOS != "linux" {
		return nil, ""
	}
	content, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return nil, "rotas locais indisponiveis"
	}
	return parseProcNetRoute(string(content)), ""
}

func readResolvConfDNSServers(path string) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseResolvConfNameServers(string(content)), nil
}

func parseResolvConfNameServers(content string) []string {
	lines := strings.Split(content, "\n")
	servers := make([]string, 0, 6)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if !strings.EqualFold(fields[0], "nameserver") {
			continue
		}
		server := strings.TrimSpace(fields[1])
		if server == "" {
			continue
		}
		servers = append(servers, server)
	}
	return uniqueStrings(servers, 12)
}

func parseProcNetRoute(content string) []localRouteRow {
	lines := strings.Split(content, "\n")
	if len(lines) <= 1 {
		return nil
	}
	out := make([]localRouteRow, 0, len(lines)-1)
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}
		iface := strings.TrimSpace(fields[0])
		destination := parseLinuxRouteHexIPv4(fields[1])
		gateway := parseLinuxRouteHexIPv4(fields[2])
		mask := parseLinuxRouteHexIPv4(fields[7])
		if iface == "" || destination == "" || mask == "" {
			continue
		}
		flags, _ := strconv.ParseUint(strings.TrimSpace(fields[3]), 16, 32)
		metric, _ := strconv.Atoi(strings.TrimSpace(fields[6]))
		if flags&0x1 == 0 {
			continue
		}
		isDefault := destination == "0.0.0.0" && mask == "0.0.0.0"
		out = append(out, localRouteRow{
			Interface:     iface,
			Destination:   destination,
			Mask:          mask,
			Gateway:       gateway,
			Metric:        metric,
			IsDefault:     isDefault,
			Protocol:      "kernel",
			AddressFamily: "ipv4",
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDefault != out[j].IsDefault {
			return out[i].IsDefault
		}
		if out[i].Metric != out[j].Metric {
			return out[i].Metric < out[j].Metric
		}
		if out[i].Interface != out[j].Interface {
			return out[i].Interface < out[j].Interface
		}
		return out[i].Destination < out[j].Destination
	})
	if len(out) > 256 {
		out = out[:256]
	}
	return out
}

func parseLinuxRouteHexIPv4(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(strings.ToLower(value), "0x")
	if value == "" {
		return ""
	}
	parsed, err := strconv.ParseUint(value, 16, 32)
	if err != nil {
		return ""
	}
	ip := net.IPv4(byte(parsed), byte(parsed>>8), byte(parsed>>16), byte(parsed>>24))
	return ip.String()
}

func collectLocalNeighbors() ([]localNeighRow, []string) {
	if runtime.GOOS != "linux" {
		return nil, nil
	}
	warnings := make([]string, 0, 1)
	neighbors := make([]localNeighRow, 0, 48)

	arpContent, arpErr := os.ReadFile("/proc/net/arp")
	if arpErr != nil {
		warnings = append(warnings, "arp local indisponivel")
	} else {
		neighbors = append(neighbors, parseProcNetARP(string(arpContent))...)
	}

	if ndpContent, ndpErr := os.ReadFile("/proc/net/ndisc_cache"); ndpErr == nil {
		neighbors = append(neighbors, parseProcNetNDiscCache(string(ndpContent))...)
	}

	deduped := make([]localNeighRow, 0, len(neighbors))
	seen := make(map[string]struct{}, len(neighbors))
	for _, neighbor := range neighbors {
		key := strings.ToLower(strings.TrimSpace(neighbor.AddressFamily)) + "|" +
			strings.ToLower(strings.TrimSpace(neighbor.Interface)) + "|" +
			strings.ToLower(strings.TrimSpace(neighbor.IP)) + "|" +
			strings.ToLower(strings.TrimSpace(neighbor.MAC))
		if key == "|||" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, neighbor)
	}
	sort.Slice(deduped, func(i, j int) bool {
		if deduped[i].Interface != deduped[j].Interface {
			return deduped[i].Interface < deduped[j].Interface
		}
		if deduped[i].AddressFamily != deduped[j].AddressFamily {
			return deduped[i].AddressFamily < deduped[j].AddressFamily
		}
		return deduped[i].IP < deduped[j].IP
	})
	if len(deduped) > 512 {
		deduped = deduped[:512]
	}
	return deduped, warnings
}

func parseProcNetARP(content string) []localNeighRow {
	lines := strings.Split(content, "\n")
	if len(lines) <= 1 {
		return nil
	}
	out := make([]localNeighRow, 0, len(lines)-1)
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		ip := strings.TrimSpace(fields[0])
		if net.ParseIP(ip) == nil {
			continue
		}
		flagsRaw := strings.TrimSpace(fields[2])
		flagsValue, _ := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(flagsRaw), "0x"), 16, 32)
		state := "incomplete"
		if flagsValue&0x2 != 0 {
			state = "reachable"
		}
		mac := strings.TrimSpace(fields[3])
		if strings.EqualFold(mac, "(incomplete)") {
			mac = ""
		}
		out = append(out, localNeighRow{
			IP:            ip,
			MAC:           mac,
			Interface:     strings.TrimSpace(fields[5]),
			State:         state,
			AddressFamily: "ipv4",
		})
	}
	return out
}

func parseProcNetNDiscCache(content string) []localNeighRow {
	lines := strings.Split(content, "\n")
	out := make([]localNeighRow, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		ip := strings.TrimSpace(fields[0])
		ipParsed := net.ParseIP(ip)
		if ipParsed == nil || !strings.Contains(ip, ":") {
			continue
		}
		neighbor := localNeighRow{
			IP:            ipParsed.String(),
			AddressFamily: "ipv6",
		}
		for i := 1; i < len(fields); i++ {
			token := strings.ToLower(strings.TrimSpace(fields[i]))
			if token == "dev" && i+1 < len(fields) {
				neighbor.Interface = strings.TrimSpace(fields[i+1])
				i++
				continue
			}
			if token == "lladdr" && i+1 < len(fields) {
				neighbor.MAC = strings.TrimSpace(fields[i+1])
				i++
				continue
			}
		}
		if neighbor.State == "" {
			neighbor.State = strings.ToLower(strings.TrimSpace(fields[len(fields)-1]))
		}
		out = append(out, neighbor)
	}
	return out
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

func isResetLikeState(state string) bool {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "CLOSE", "CLOSED", "CLOSE_WAIT", "CLOSING", "LAST_ACK":
		return true
	default:
		return false
	}
}

func isTimeoutLikeState(state string) bool {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "SYN_SENT", "SYN_RECV", "FIN_WAIT1", "FIN_WAIT2", "TIME_WAIT":
		return true
	default:
		return false
	}
}

func estimateAvgDurationSec(firstSeen, lastSeen int64, sampleInterval time.Duration) float64 {
	if firstSeen <= 0 || lastSeen <= 0 || lastSeen < firstSeen {
		return 0
	}
	spanSec := float64(lastSeen-firstSeen) + sampleInterval.Seconds()
	if spanSec < sampleInterval.Seconds() {
		spanSec = sampleInterval.Seconds()
	}
	return spanSec
}

func maxInt(a, b int) int {
	if a >= b {
		return a
	}
	return b
}

func averageDuration(weighted, weight float64) float64 {
	if weight <= 0 {
		return 0
	}
	return weighted / weight
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

type networkRisk struct {
	problematic bool
	reasons     []string
	tags        []string
}

func mergeNetworkRisk(problematic *bool, reasons map[string]struct{}, tags map[string]struct{}, risk networkRisk) {
	if problematic != nil && risk.problematic {
		*problematic = true
	}
	if reasons != nil {
		for _, reason := range risk.reasons {
			reasons[reason] = struct{}{}
		}
	}
	if tags != nil {
		for _, tag := range risk.tags {
			tags[tag] = struct{}{}
		}
	}
}

func classifyNetworkRisk(remotePort uint32, remoteScope, direction, protocol string) networkRisk {
	scope := strings.ToLower(strings.TrimSpace(remoteScope))
	dir := strings.ToLower(strings.TrimSpace(direction))
	proto := strings.ToLower(strings.TrimSpace(protocol))
	isPublic := scope == "public"
	isUnknownScope := scope == "" || scope == "unknown"
	isUnknownDirection := dir == "" || dir == "unknown"
	_, isSensitivePort := highRiskPorts[remotePort]
	_, isLegacyPlaintext := legacyPlaintextPorts[remotePort]
	isUncommonProtocol := proto != "tcp" && proto != "udp"

	reasons := make([]string, 0, 4)
	tags := make([]string, 0, 4)

	if isPublic {
		tags = append(tags, "public_exposure")
	}
	if isSensitivePort {
		tags = append(tags, "sensitive_port")
	}
	if isUnknownDirection {
		tags = append(tags, "unknown_direction")
	}
	if isUncommonProtocol {
		tags = append(tags, "uncommon_protocol")
	}

	if isPublic && isSensitivePort {
		reasons = append(reasons, "porta sensível em escopo público")
	} else if (isPublic || isUnknownScope) && remotePort > 0 && isSensitivePort {
		reasons = append(reasons, "porta sensível em escopo não confiável")
	}
	if isLegacyPlaintext {
		reasons = append(reasons, "porta legada sem criptografia forte")
	}
	if isUnknownDirection {
		reasons = append(reasons, "direção de tráfego indefinida")
	}
	if isUncommonProtocol {
		reasons = append(reasons, "protocolo incomum")
	}

	return networkRisk{
		problematic: len(reasons) > 0,
		reasons:     uniqueStrings(reasons, 5),
		tags:        uniqueStrings(tags, 6),
	}
}

func classifyNetworkFailureRisk(
	remotePort uint32,
	remoteScope, direction, protocol, state, dnsQuery, dnsAnswer string,
	samples, resetHits, timeoutHits int,
) networkRisk {
	scope := strings.ToLower(strings.TrimSpace(remoteScope))
	dir := strings.ToLower(strings.TrimSpace(direction))
	proto := strings.ToLower(strings.TrimSpace(protocol))
	stateValue := strings.ToUpper(strings.TrimSpace(state))
	query := strings.TrimSpace(dnsQuery)
	answer := strings.TrimSpace(dnsAnswer)
	isPublic := scope == "public"
	isUnknownScope := scope == "" || scope == "unknown"
	_, isAdminPort := adminPorts[remotePort]
	_, isDNSPort := dnsServicePorts[remotePort]
	_, isHandshakeNoSession := handshakeNoSessionStates[stateValue]

	sampleCount := maxInt(samples, 1)
	timeoutRate := float64(timeoutHits) / float64(sampleCount)
	resetRate := float64(resetHits) / float64(sampleCount)
	unstableRate := timeoutRate + resetRate
	hasUnknownDirection := dir == "" || dir == "unknown"

	reasons := make([]string, 0, 4)
	tags := make([]string, 0, 4)

	if isAdminPort && (isPublic || isUnknownScope) {
		if isPublic {
			reasons = append(reasons, "porta administrativa exposta em escopo público")
		} else {
			reasons = append(reasons, "porta administrativa exposta em escopo não confiável")
		}
		tags = append(tags, "admin_port_exposure")
	}

	if isDNSPort && samples >= 3 {
		if timeoutHits >= 2 || timeoutRate >= 0.4 {
			reasons = append(reasons, "dns instável com timeout recorrente")
			tags = append(tags, "dns_instability")
		} else if query != "" && answer == "" {
			reasons = append(reasons, "dns instável sem resposta consistente")
			tags = append(tags, "dns_instability")
		}
	}

	if (isPublic || isUnknownScope) && samples >= 3 && query == "" && answer != "" {
		reasons = append(reasons, "tráfego para IP público sem contexto de reputação")
		tags = append(tags, "unknown_reputation_ip")
	}

	if proto == "tcp" && isHandshakeNoSession && samples >= 3 {
		if timeoutHits >= 2 || resetHits >= 2 || unstableRate >= 0.4 || hasUnknownDirection {
			reasons = append(reasons, "handshake recorrente sem sessão estabelecida")
			tags = append(tags, "failed_handshake")
		}
	}

	return networkRisk{
		problematic: len(reasons) > 0,
		reasons:     uniqueStrings(reasons, 5),
		tags:        uniqueStrings(tags, 6),
	}
}

func uniqueStrings(items []string, limit int) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		normalized := strings.TrimSpace(item)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func sortedSetValues(set map[string]struct{}, limit int) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for value := range set {
		v := strings.TrimSpace(value)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	sort.Strings(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func joinSortedSet(set map[string]struct{}, sep string) string {
	values := sortedSetValues(set, 0)
	if len(values) == 0 {
		return ""
	}
	return strings.Join(values, sep)
}

func resolveProcessMeta(ctx context.Context, pid int32) processMeta {
	meta := processMeta{}
	if pid <= 0 {
		return meta
	}
	p, err := process.NewProcessWithContext(ctx, pid)
	if err != nil {
		return meta
	}
	if name, e := p.NameWithContext(ctx); e == nil {
		meta.name = strings.TrimSpace(name)
	}
	if parentID, e := p.PpidWithContext(ctx); e == nil {
		meta.parentID = parentID
	}
	if username, e := p.UsernameWithContext(ctx); e == nil {
		meta.username = strings.TrimSpace(username)
	}
	if exe, e := p.ExeWithContext(ctx); e == nil {
		meta.exe = strings.TrimSpace(exe)
	}
	if cmdline, e := p.CmdlineWithContext(ctx); e == nil {
		cmdline = strings.Join(strings.Fields(strings.TrimSpace(cmdline)), " ")
		if len(cmdline) > maxCmdlineLength {
			cmdline = cmdline[:maxCmdlineLength]
		}
		meta.cmdline = cmdline
	}
	meta.service = resolveServiceName(pid, meta.name, meta.exe)
	return meta
}

func resolveServiceName(pid int32, processName, processExe string) string {
	if fromCgroup := serviceNameFromCgroup(pid); fromCgroup != "" {
		return fromCgroup
	}
	base := strings.TrimSpace(processName)
	if base == "" {
		base = strings.TrimSpace(processExe)
		if base != "" {
			if i := strings.LastIndexAny(base, "/\\"); i >= 0 && i < len(base)-1 {
				base = base[i+1:]
			}
		}
	}
	if len(base) > maxServiceNameLen {
		base = base[:maxServiceNameLen]
	}
	return base
}

func serviceNameFromCgroup(pid int32) string {
	if pid <= 0 {
		return ""
	}
	path := "/proc/" + strconv.FormatInt(int64(pid), 10) + "/cgroup"
	// Mountpoint inexistente fora de Linux; nesse caso retorna vazio.
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return parseServiceNameFromCgroup(string(content))
}

func parseServiceNameFromCgroup(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, ".service") {
			continue
		}
		fields := strings.Split(line, "/")
		for i := len(fields) - 1; i >= 0; i-- {
			part := strings.TrimSpace(fields[i])
			if part == "" || !strings.HasSuffix(part, ".service") {
				continue
			}
			if len(part) > maxServiceNameLen {
				part = part[:maxServiceNameLen]
			}
			return part
		}
	}
	return ""
}

func resolveReverseDNSWithCache(ctx context.Context, ipRaw string) string {
	ip := strings.TrimSpace(ipRaw)
	if ip == "" {
		return ""
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}
	normalizedIP := parsed.String()
	now := time.Now()

	reverseDNSCache.mu.Lock()
	if cached, ok := reverseDNSCache.entries[normalizedIP]; ok && now.Before(cached.expiresAt) {
		reverseDNSCache.mu.Unlock()
		return cached.host
	}
	reverseDNSCache.mu.Unlock()

	lookupCtx, cancel := context.WithTimeout(ctx, reverseDNSTimeout)
	defer cancel()

	names, err := net.DefaultResolver.LookupAddr(lookupCtx, normalizedIP)
	host := pickReverseDNSName(names)
	ttl := reverseDNSCacheTTL
	if err != nil || host == "" {
		host = ""
		ttl = reverseDNSFailTTL
	}

	reverseDNSCache.mu.Lock()
	if len(reverseDNSCache.entries) >= maxReverseDNSCache {
		for key, entry := range reverseDNSCache.entries {
			if now.After(entry.expiresAt) {
				delete(reverseDNSCache.entries, key)
			}
		}
		if len(reverseDNSCache.entries) >= maxReverseDNSCache {
			for key := range reverseDNSCache.entries {
				delete(reverseDNSCache.entries, key)
				break
			}
		}
	}
	reverseDNSCache.entries[normalizedIP] = reverseDNSCacheEntry{
		host:      host,
		expiresAt: now.Add(ttl),
	}
	reverseDNSCache.mu.Unlock()

	return host
}

func pickReverseDNSName(candidates []string) string {
	for _, candidate := range candidates {
		normalized := normalizeReverseDNSName(candidate)
		if normalized != "" {
			return normalized
		}
	}
	return ""
}

func normalizeReverseDNSName(value string) string {
	host := strings.ToLower(strings.TrimSpace(value))
	host = strings.Trim(host, ". \t\n\r")
	if host == "" {
		return ""
	}
	if idx := strings.Index(host, "%"); idx > 0 {
		host = host[:idx]
	}
	return host
}

func inferResolutionContext(cmdline, reverseDNS, remoteIP string, remotePort uint32, protocol string) (dnsQuery, dnsAnswer, sni, tlsSubject string) {
	remote := strings.TrimSpace(remoteIP)
	reverse := normalizeReverseDNSName(reverseDNS)
	cmdlineHost := extractHostFromCmdline(cmdline)

	dnsQuery = cmdlineHost
	if dnsQuery == "" {
		dnsQuery = reverse
	}
	dnsQuery = normalizeResolutionHost(dnsQuery)

	if remote != "" {
		dnsAnswer = trimToLen(remote, maxResolutionLen)
	}

	if isLikelyTLSEndpoint(protocol, remotePort) {
		if dnsQuery != "" {
			sni = dnsQuery
		}
		if reverse != "" {
			tlsSubject = trimToLen(reverse, maxTLSSubjectLen)
		} else if sni != "" {
			tlsSubject = trimToLen(sni, maxTLSSubjectLen)
		}
	}

	return
}

func extractHostFromCmdline(cmdline string) string {
	line := strings.TrimSpace(cmdline)
	if line == "" {
		return ""
	}
	if matches := urlHostRegex.FindStringSubmatch(line); len(matches) >= 2 {
		if host := normalizeResolutionHost(matches[1]); host != "" {
			return host
		}
	}
	for _, tokenRaw := range strings.Fields(line) {
		token := strings.Trim(tokenRaw, "\"'`,;()[]{}")
		if token == "" {
			continue
		}
		if strings.Contains(token, "://") {
			if matches := urlHostRegex.FindStringSubmatch(token); len(matches) >= 2 {
				if host := normalizeResolutionHost(matches[1]); host != "" {
					return host
				}
			}
		}
		if idx := strings.Index(token, "="); idx > 0 && idx < len(token)-1 {
			token = token[idx+1:]
		}
		hostCandidate := strings.Trim(token, "\"'`,;()[]{}")
		if at := strings.LastIndex(hostCandidate, "@"); at >= 0 && at < len(hostCandidate)-1 {
			hostCandidate = hostCandidate[at+1:]
		}
		if slash := strings.IndexAny(hostCandidate, "/?"); slash >= 0 {
			hostCandidate = hostCandidate[:slash]
		}
		hostCandidate = strings.TrimSpace(hostCandidate)
		if hostCandidate == "" {
			continue
		}
		if host, port, err := net.SplitHostPort(hostCandidate); err == nil {
			if _, convErr := strconv.Atoi(port); convErr == nil {
				hostCandidate = host
			}
		} else if strings.Count(hostCandidate, ":") == 1 && !strings.Contains(hostCandidate, "]") {
			parts := strings.SplitN(hostCandidate, ":", 2)
			if len(parts) == 2 {
				if _, convErr := strconv.Atoi(parts[1]); convErr == nil {
					hostCandidate = parts[0]
				}
			}
		}
		if host := normalizeResolutionHost(hostCandidate); host != "" {
			return host
		}
	}
	return ""
}

func normalizeResolutionHost(raw string) string {
	host := normalizeReverseDNSName(raw)
	if host == "" {
		return ""
	}
	if parsed := net.ParseIP(host); parsed != nil {
		return ""
	}
	if !strings.Contains(host, ".") {
		return ""
	}
	parts := strings.Split(host, ".")
	for _, part := range parts {
		if part == "" || len(part) > 63 {
			return ""
		}
		if strings.HasPrefix(part, "-") || strings.HasSuffix(part, "-") {
			return ""
		}
		for _, ch := range part {
			if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
				continue
			}
			return ""
		}
	}
	return trimToLen(host, maxResolutionLen)
}

func isLikelyTLSEndpoint(protocol string, remotePort uint32) bool {
	if strings.ToLower(strings.TrimSpace(protocol)) != "tcp" {
		return false
	}
	_, ok := tlsLikelyPorts[remotePort]
	return ok
}

func trimToLen(value string, maxLen int) string {
	s := strings.TrimSpace(value)
	if s == "" || maxLen <= 0 {
		return ""
	}
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}
