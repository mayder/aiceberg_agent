package networkcapture

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
)

func TestInferDirection(t *testing.T) {
	tests := []struct {
		name       string
		state      string
		localPort  uint32
		remotePort uint32
		want       string
	}{
		{name: "syn sent", state: "SYN_SENT", localPort: 54000, remotePort: 443, want: "egress"},
		{name: "syn recv", state: "SYN_RECV", localPort: 443, remotePort: 54000, want: "ingress"},
		{name: "listen", state: "LISTEN", localPort: 22, remotePort: 0, want: "listen"},
		{name: "ephemeral local", state: "ESTABLISHED", localPort: 55000, remotePort: 443, want: "egress"},
		{name: "linux ephemeral local", state: "ESTABLISHED", localPort: 32780, remotePort: 443, want: "egress"},
		{name: "ephemeral remote", state: "ESTABLISHED", localPort: 443, remotePort: 55000, want: "ingress"},
		{name: "same ports", state: "ESTABLISHED", localPort: 8080, remotePort: 8080, want: "lateral"},
		{name: "fallback", state: "ESTABLISHED", localPort: 443, remotePort: 80, want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferDirection(tt.state, tt.localPort, tt.remotePort)
			if got != tt.want {
				t.Fatalf("inferDirection() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSafeDelta(t *testing.T) {
	if got := safeDelta(100, 90); got != 10 {
		t.Fatalf("safeDelta positive = %d, want 10", got)
	}
	if got := safeDelta(90, 100); got != 0 {
		t.Fatalf("safeDelta negative = %d, want 0", got)
	}
}

func TestApplyEstimatedTrafficPropagatesToPeers(t *testing.T) {
	flowOneKey := flowKey{
		proto:      "tcp",
		direction:  "egress",
		remoteIP:   "198.51.100.10",
		remotePort: 443,
	}
	flowTwoKey := flowKey{
		proto:      "tcp",
		direction:  "ingress",
		remoteIP:   "198.51.100.20",
		remotePort: 8443,
	}
	flows := map[flowKey]*flowAgg{
		flowOneKey: {samples: 2, scope: "public"},
		flowTwoKey: {samples: 1, scope: "private"},
	}
	peers := map[peerKey]*peerAgg{
		{proto: "tcp", direction: "egress", remoteIP: "198.51.100.10", remotePort: 443, scope: "public"}:    {},
		{proto: "tcp", direction: "ingress", remoteIP: "198.51.100.20", remotePort: 8443, scope: "private"}: {},
	}

	applyEstimatedTraffic(flows, peers, []ifaceDelta{
		{
			BytesSent:   3000,
			BytesRecv:   1500,
			PacketsSent: 300,
			PacketsRecv: 150,
		},
	})

	if flows[flowOneKey].bytesOut <= 0 || flows[flowOneKey].packetsOut <= 0 {
		t.Fatalf("expected egress flow to receive estimated outbound traffic, got bytes_out=%d packets_out=%d", flows[flowOneKey].bytesOut, flows[flowOneKey].packetsOut)
	}
	if flows[flowTwoKey].bytesIn <= 0 || flows[flowTwoKey].packetsIn <= 0 {
		t.Fatalf("expected ingress flow to receive estimated inbound traffic, got bytes_in=%d packets_in=%d", flows[flowTwoKey].bytesIn, flows[flowTwoKey].packetsIn)
	}

	peerOne := peers[peerKey{proto: "tcp", direction: "egress", remoteIP: "198.51.100.10", remotePort: 443, scope: "public"}]
	if peerOne.bytesOut != flows[flowOneKey].bytesOut || peerOne.packetsOut != flows[flowOneKey].packetsOut {
		t.Fatalf("peer egress traffic mismatch: peer bytes_out=%d packets_out=%d flow bytes_out=%d packets_out=%d", peerOne.bytesOut, peerOne.packetsOut, flows[flowOneKey].bytesOut, flows[flowOneKey].packetsOut)
	}
	peerTwo := peers[peerKey{proto: "tcp", direction: "ingress", remoteIP: "198.51.100.20", remotePort: 8443, scope: "private"}]
	if peerTwo.bytesIn != flows[flowTwoKey].bytesIn || peerTwo.packetsIn != flows[flowTwoKey].packetsIn {
		t.Fatalf("peer ingress traffic mismatch: peer bytes_in=%d packets_in=%d flow bytes_in=%d packets_in=%d", peerTwo.bytesIn, peerTwo.packetsIn, flows[flowTwoKey].bytesIn, flows[flowTwoKey].packetsIn)
	}
}

func TestParseServiceNameFromCgroup(t *testing.T) {
	content := "0::/user.slice/user-1000.slice/user@1000.service/app.slice/sshd.service\n"
	if got := parseServiceNameFromCgroup(content); got != "sshd.service" {
		t.Fatalf("parseServiceNameFromCgroup() = %q, want %q", got, "sshd.service")
	}
}

func TestParseServiceNameFromCgroupNoService(t *testing.T) {
	content := "0::/user.slice/user-1000.slice/session-3.scope\n"
	if got := parseServiceNameFromCgroup(content); got != "" {
		t.Fatalf("parseServiceNameFromCgroup() = %q, want empty", got)
	}
}

func TestNormalizeReverseDNSName(t *testing.T) {
	got := normalizeReverseDNSName(" API.Example.COM. ")
	if got != "api.example.com" {
		t.Fatalf("normalizeReverseDNSName() = %q, want %q", got, "api.example.com")
	}
}

func TestPickReverseDNSName(t *testing.T) {
	got := pickReverseDNSName([]string{"", "  ", "db.interno.local. "})
	if got != "db.interno.local" {
		t.Fatalf("pickReverseDNSName() = %q, want %q", got, "db.interno.local")
	}
}

func TestClassifyNetworkRiskSensitivePublic(t *testing.T) {
	risk := classifyNetworkRisk(3389, "public", "unknown", "tcp")
	if !risk.problematic {
		t.Fatalf("expected problematic=true")
	}
	if len(risk.reasons) == 0 {
		t.Fatalf("expected reasons")
	}
	if len(risk.tags) == 0 {
		t.Fatalf("expected tags")
	}
}

func TestClassifyNetworkRiskSafeFlow(t *testing.T) {
	risk := classifyNetworkRisk(443, "private", "egress", "tcp")
	if risk.problematic {
		t.Fatalf("expected problematic=false")
	}
	if len(risk.reasons) != 0 {
		t.Fatalf("expected no reasons, got %v", risk.reasons)
	}
}

func TestClassifyNetworkRiskUncommonProtocol(t *testing.T) {
	risk := classifyNetworkRisk(0, "private", "egress", "ip")
	if !risk.problematic {
		t.Fatalf("expected uncommon protocol to be problematic")
	}
	found := false
	for _, tag := range risk.tags {
		if tag == "uncommon_protocol" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected tag uncommon_protocol, got %v", risk.tags)
	}
}

func TestClassifyNetworkFailureRiskAdminPortPublic(t *testing.T) {
	risk := classifyNetworkFailureRisk(22, "public", "egress", "tcp", "ESTABLISHED", "", "", 5, 0, 0)
	if !risk.problematic {
		t.Fatalf("expected admin public exposure as problematic")
	}
	foundReason := false
	for _, reason := range risk.reasons {
		if reason == "porta administrativa exposta em escopo público" {
			foundReason = true
			break
		}
	}
	if !foundReason {
		t.Fatalf("expected admin exposure reason, got %v", risk.reasons)
	}
}

func TestClassifyNetworkFailureRiskDNSInstability(t *testing.T) {
	risk := classifyNetworkFailureRisk(53, "private", "egress", "udp", "NONE", "dns.corp.local", "10.0.0.53", 6, 0, 3)
	if !risk.problematic {
		t.Fatalf("expected dns instability as problematic")
	}
	foundReason := false
	for _, reason := range risk.reasons {
		if reason == "dns instável com timeout recorrente" {
			foundReason = true
			break
		}
	}
	if !foundReason {
		t.Fatalf("expected dns instability reason, got %v", risk.reasons)
	}
}

func TestClassifyNetworkFailureRiskFailedHandshake(t *testing.T) {
	risk := classifyNetworkFailureRisk(443, "public", "egress", "tcp", "SYN_SENT", "api.example.com", "203.0.113.10", 5, 0, 3)
	if !risk.problematic {
		t.Fatalf("expected failed handshake as problematic")
	}
	foundReason := false
	for _, reason := range risk.reasons {
		if reason == "handshake recorrente sem sessão estabelecida" {
			foundReason = true
			break
		}
	}
	if !foundReason {
		t.Fatalf("expected failed handshake reason, got %v", risk.reasons)
	}
}

func TestClassifyNetworkFailureRiskUnknownReputationIP(t *testing.T) {
	risk := classifyNetworkFailureRisk(443, "public", "egress", "tcp", "ESTABLISHED", "", "203.0.113.55", 4, 0, 0)
	if !risk.problematic {
		t.Fatalf("expected unknown public ip reputation as problematic")
	}
	foundReason := false
	for _, reason := range risk.reasons {
		if reason == "tráfego para IP público sem contexto de reputação" {
			foundReason = true
			break
		}
	}
	if !foundReason {
		t.Fatalf("expected unknown reputation reason, got %v", risk.reasons)
	}
}

func TestParseResolvConfNameServers(t *testing.T) {
	content := `
# comment
nameserver 10.0.0.53
search corp.local
nameserver 1.1.1.1
`
	got := parseResolvConfNameServers(content)
	if len(got) != 2 {
		t.Fatalf("expected 2 dns servers, got %d (%v)", len(got), got)
	}
	if got[0] != "10.0.0.53" || got[1] != "1.1.1.1" {
		t.Fatalf("unexpected dns list: %v", got)
	}
}

func TestParseProcNetRoute(t *testing.T) {
	content := `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
eth0	00000000	0101A8C0	0003	0	0	100	00000000	0	0	0
eth0	0001A8C0	00000000	0001	0	0	100	00FFFFFF	0	0	0
`
	routes := parseProcNetRoute(content)
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}
	if !routes[0].IsDefault {
		t.Fatalf("expected first route to be default")
	}
	if routes[0].Gateway != "192.168.1.1" {
		t.Fatalf("unexpected gateway: %q", routes[0].Gateway)
	}
}

func TestParseProcNetARP(t *testing.T) {
	content := `IP address       HW type     Flags       HW address            Mask     Device
192.168.1.10     0x1         0x2         aa:bb:cc:dd:ee:ff     *        eth0
`
	neighbors := parseProcNetARP(content)
	if len(neighbors) != 1 {
		t.Fatalf("expected 1 neighbor, got %d", len(neighbors))
	}
	if neighbors[0].IP != "192.168.1.10" {
		t.Fatalf("unexpected ip: %q", neighbors[0].IP)
	}
	if neighbors[0].AddressFamily != "ipv4" {
		t.Fatalf("unexpected family: %q", neighbors[0].AddressFamily)
	}
}

func TestBuildSocketSnapshotAggregatesBySocketTuple(t *testing.T) {
	c := &collector{maxFlows: 50}
	flows := map[flowKey]*flowAgg{
		{
			proto:      "tcp",
			direction:  "egress",
			localIP:    "10.0.0.10",
			localPort:  51000,
			remoteIP:   "8.8.8.8",
			remotePort: 53,
			process:    "proc-a",
		}: {
			state:     "ESTABLISHED",
			samples:   2,
			firstSeen: 100,
			lastSeen:  140,
		},
		{
			proto:      "tcp",
			direction:  "egress",
			localIP:    "10.0.0.10",
			localPort:  51000,
			remoteIP:   "8.8.8.8",
			remotePort: 53,
			process:    "proc-b",
		}: {
			state:     "ESTABLISHED",
			samples:   3,
			firstSeen: 90,
			lastSeen:  180,
		},
		{
			proto:      "udp",
			direction:  "egress",
			localIP:    "10.0.0.10",
			localPort:  53000,
			remoteIP:   "1.1.1.1",
			remotePort: 53,
			process:    "proc-c",
		}: {
			state:     "NONE",
			samples:   1,
			firstSeen: 120,
			lastSeen:  125,
		},
	}

	got := c.buildSocketSnapshot(flows, time.Second, 50)
	if len(got) != 2 {
		t.Fatalf("expected 2 snapshot rows, got %d", len(got))
	}
	if got[0].Protocol != "tcp" || got[0].Direction != "egress" {
		t.Fatalf("unexpected first row protocol/direction: %#v", got[0])
	}
	if got[0].Samples != 5 {
		t.Fatalf("expected tcp samples=5, got %d", got[0].Samples)
	}
	if got[0].FirstSeenUnix != 90 || got[0].LastSeenUnix != 180 {
		t.Fatalf("unexpected first/last seen: first=%d last=%d", got[0].FirstSeenUnix, got[0].LastSeenUnix)
	}
	if got[0].State != "ESTABLISHED" {
		t.Fatalf("unexpected state: %q", got[0].State)
	}
}

func TestBuildServiceMapInfersNonInstrumentedServiceAndDBDependency(t *testing.T) {
	advanced := &passiveAdvancedPayload{
		AppliedMode: "socket_snapshot",
		Capabilities: passiveCapabilities{
			Netlink: true,
			PCAP:    false,
			EBPF:    false,
		},
	}
	payload := buildServiceMapPayload([]flowRow{
		{
			Protocol:      "tcp",
			Direction:     "egress",
			LocalIP:       "10.0.0.10",
			LocalPort:     48000,
			RemoteIP:      "10.0.0.20",
			RemotePort:    5432,
			RemoteScope:   "private",
			State:         "ESTABLISHED",
			Process:       "legacy-api",
			DNSQuery:      "db.internal.local",
			ServiceName:   "",
			Samples:       3,
			BytesIn:       1200,
			BytesOut:      2400,
			FirstSeenUnix: 100,
			LastSeenUnix:  120,
		},
	}, nil, advanced, passiveCollectOptions{
		requestedMode:   "auto",
		advancedEnabled: true,
		usmEnabled:      true,
	})

	if payload == nil || !payload.Enabled {
		t.Fatalf("expected enabled service map")
	}
	var foundService bool
	for _, service := range payload.Services {
		if service.Service == "legacy-api" && service.Confidence == "inferred" {
			foundService = true
			break
		}
	}
	if !foundService {
		t.Fatalf("expected inferred legacy-api service, got %#v", payload.Services)
	}
	if len(payload.Dependencies) != 1 {
		t.Fatalf("expected one dependency, got %#v", payload.Dependencies)
	}
	dep := payload.Dependencies[0]
	if dep.SourceService != "legacy-api" || dep.TargetService != "db.internal.local" || dep.TargetType != "database" {
		t.Fatalf("unexpected dependency: %#v", dep)
	}
	if dep.Confidence != "confirmed" {
		t.Fatalf("expected confirmed dependency from repeated flow with DNS, got %q", dep.Confidence)
	}
	if payload.SystemProbe.EBPFActive {
		t.Fatalf("ebpf must not be active without applied ebpf mode")
	}
	if payload.SystemProbe.Fallback == "" {
		t.Fatalf("expected fallback status")
	}
}

func TestBuildNetworkPerfAndWorkloadSecuritySignals(t *testing.T) {
	flows := []flowRow{
		{
			Protocol:    "tcp",
			Direction:   "egress",
			RemoteIP:    "203.0.113.10",
			RemotePort:  22,
			RemoteScope: "public",
			State:       "SYN_SENT",
			Process:     "nc",
			Samples:     4,
			ResetHits:   1,
			TimeoutHits: 3,
			RiskTags:    []string{"failed_handshake", "admin_port_exposure"},
		},
	}

	npm := buildNetworkPerfPayload(flows)
	if len(npm.TopTalkers) != 1 {
		t.Fatalf("expected top talker, got %#v", npm.TopTalkers)
	}
	if npm.TopTalkers[0].RemoteIP != "203.0.113.0/24" {
		t.Fatalf("expected public ip mask, got %q", npm.TopTalkers[0].RemoteIP)
	}
	if len(npm.ExposedAdminPorts) != 1 {
		t.Fatalf("expected admin exposure, got %#v", npm.ExposedAdminPorts)
	}

	workload := buildWorkloadSecurityPayload(flows)
	if workload.DestructiveAction {
		t.Fatalf("workload security must not perform destructive action")
	}
	if len(workload.Signals) < 2 {
		t.Fatalf("expected security signals, got %#v", workload.Signals)
	}
	for _, signal := range workload.Signals {
		if signal.RemoteIP == "203.0.113.10" {
			t.Fatalf("expected masked remote ip in signal: %#v", signal)
		}
	}
}

func TestExtractHostFromCmdlineURL(t *testing.T) {
	cmdline := `curl --max-time 3 https://api.example.com:443/health`
	got := extractHostFromCmdline(cmdline)
	if got != "api.example.com" {
		t.Fatalf("extractHostFromCmdline() = %q, want %q", got, "api.example.com")
	}
}

func TestInferResolutionContextTLS(t *testing.T) {
	dnsQuery, dnsAnswer, sni, tlsSubject := inferResolutionContext(
		`wget https://edge.acme.local/login`,
		"edge.acme.local.",
		"203.0.113.11",
		443,
		"tcp",
	)

	if dnsQuery != "edge.acme.local" {
		t.Fatalf("dns_query = %q, want %q", dnsQuery, "edge.acme.local")
	}
	if dnsAnswer != "203.0.113.11" {
		t.Fatalf("dns_answer = %q, want %q", dnsAnswer, "203.0.113.11")
	}
	if sni != "edge.acme.local" {
		t.Fatalf("sni = %q, want %q", sni, "edge.acme.local")
	}
	if tlsSubject != "edge.acme.local" {
		t.Fatalf("tls_subject = %q, want %q", tlsSubject, "edge.acme.local")
	}
}

func TestConnectionQualityHelpers(t *testing.T) {
	if !isResetLikeState("CLOSE_WAIT") {
		t.Fatalf("expected CLOSE_WAIT as reset-like state")
	}
	if isResetLikeState("ESTABLISHED") {
		t.Fatalf("ESTABLISHED must not be reset-like state")
	}
	if !isTimeoutLikeState("SYN_SENT") {
		t.Fatalf("expected SYN_SENT as timeout-like state")
	}
	if isTimeoutLikeState("TIME_WAIT") {
		t.Fatalf("TIME_WAIT must not be timeout-like state")
	}
	if isTimeoutLikeState("ESTABLISHED") {
		t.Fatalf("ESTABLISHED must not be timeout-like state")
	}
}

func TestEstimateAvgDurationSec(t *testing.T) {
	got := estimateAvgDurationSec(100, 110, time.Second)
	if got != 11 {
		t.Fatalf("estimateAvgDurationSec() = %v, want 11", got)
	}
	if got = estimateAvgDurationSec(0, 10, time.Second); got != 0 {
		t.Fatalf("estimateAvgDurationSec() invalid firstSeen = %v, want 0", got)
	}
}

func TestNormalizePassiveMode(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: "auto"},
		{in: "AUTO", want: "auto"},
		{in: "socket_snapshot", want: "socket"},
		{in: "socket", want: "socket"},
		{in: "netlink", want: "netlink"},
		{in: "pcap", want: "pcap"},
		{in: "ebpf", want: "ebpf"},
		{in: "invalid-mode", want: "auto"},
	}
	for _, tt := range tests {
		if got := normalizePassiveMode(tt.in); got != tt.want {
			t.Fatalf("normalizePassiveMode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseNetlinkLinksJSON(t *testing.T) {
	raw := []byte(`[
  {"ifname":"eth0","operstate":"UP","address":"aa:bb:cc:dd:ee:ff","mtu":1500,"stats64":{"rx":{"bytes":10,"packets":2,"errors":0,"dropped":0},"tx":{"bytes":20,"packets":3,"errors":1,"dropped":2}}},
  {"ifname":"lo","operstate":"UNKNOWN","address":"00:00:00:00:00:00","mtu":65536,"stats":{"rx":{"bytes":1,"packets":1,"errors":0,"dropped":0},"tx":{"bytes":1,"packets":1,"errors":0,"dropped":0}}}
]`)
	rows := parseNetlinkLinksJSON(raw)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Name != "eth0" {
		t.Fatalf("expected first row eth0, got %q", rows[0].Name)
	}
	if rows[0].TXDrops != 2 {
		t.Fatalf("expected tx_drops=2, got %d", rows[0].TXDrops)
	}
	if rows[1].Name != "lo" {
		t.Fatalf("expected second row lo, got %q", rows[1].Name)
	}
}

func TestParseTCPDumpOutput(t *testing.T) {
	raw := `
IP 10.0.0.10.54000 > 8.8.8.8.53: UDP, length 37
IP 8.8.8.8.53 > 10.0.0.10.54000: UDP, length 85
IP 10.0.0.10.51000 > 203.0.113.11.443: Flags [S], seq 1, win 65535
3 packets captured
4 packets received by filter
1 packets dropped by kernel
`
	localIPs := map[string]struct{}{"10.0.0.10": {}}
	parsed := parseTCPDumpOutput(raw, localIPs)
	if parsed.CapturedPackets != 3 {
		t.Fatalf("captured_packets=%d, want 3", parsed.CapturedPackets)
	}
	if parsed.ReceivedByFilter != 4 {
		t.Fatalf("received_by_filter=%d, want 4", parsed.ReceivedByFilter)
	}
	if parsed.DroppedByKernel != 1 {
		t.Fatalf("dropped_by_kernel=%d, want 1", parsed.DroppedByKernel)
	}
	if len(parsed.Flows) == 0 {
		t.Fatalf("expected non-empty flow summary")
	}
	found := false
	for _, flow := range parsed.Flows {
		if flow.RemoteIP == "8.8.8.8" && flow.RemotePort == 53 && flow.Direction == "egress" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected flow to 8.8.8.8:53 in parsed flows: %#v", parsed.Flows)
	}
}

func TestParseExternalSourceSpec(t *testing.T) {
	sourceType, path := parseExternalSourceSpec("netflow:/tmp/netflow.jsonl")
	if sourceType != "netflow" {
		t.Fatalf("expected netflow type, got %q", sourceType)
	}
	if path != "/tmp/netflow.jsonl" {
		t.Fatalf("unexpected path: %q", path)
	}

	sourceType, path = parseExternalSourceSpec("/var/log/firewall.log")
	if sourceType != "firewall" {
		t.Fatalf("expected firewall type by filename, got %q", sourceType)
	}
	if path != "/var/log/firewall.log" {
		t.Fatalf("unexpected fallback path: %q", path)
	}
}

func TestParseExternalObservationLineJSON(t *testing.T) {
	line := `{"src_ip":"10.0.0.10","dst_ip":"198.51.100.20","src_port":54000,"dst_port":443,"protocol":"tcp","bytes":1200,"packets":12,"first_seen":1700000000,"last_seen":1700000010,"dns_query":"api.example.com","sni":"api.example.com"}`
	obs, ok := parseExternalObservationLine("netflow", line)
	if !ok {
		t.Fatalf("expected json observation to parse")
	}
	if obs.sourceType != "netflow" {
		t.Fatalf("unexpected source type: %q", obs.sourceType)
	}
	if obs.protocol != "tcp" {
		t.Fatalf("unexpected protocol: %q", obs.protocol)
	}
	if obs.srcIP != "10.0.0.10" || obs.dstIP != "198.51.100.20" {
		t.Fatalf("unexpected src/dst: %q -> %q", obs.srcIP, obs.dstIP)
	}
	if obs.bytes != 1200 || obs.packets != 12 {
		t.Fatalf("unexpected bytes/packets: %d/%d", obs.bytes, obs.packets)
	}
}

func TestParseExternalObservationLineKV(t *testing.T) {
	line := `src_ip=10.0.0.10 dst_ip=198.51.100.20 src_port=53000 dst_port=53 protocol=udp packets=6 bytes=600`
	obs, ok := parseExternalObservationLine("siem", line)
	if !ok {
		t.Fatalf("expected kv observation to parse")
	}
	if obs.protocol != "udp" {
		t.Fatalf("unexpected protocol: %q", obs.protocol)
	}
	if obs.srcPort != 53000 || obs.dstPort != 53 {
		t.Fatalf("unexpected ports: %d -> %d", obs.srcPort, obs.dstPort)
	}
}

func TestMergeExternalObservation(t *testing.T) {
	flows := make(map[flowKey]*flowAgg)
	peers := make(map[peerKey]*peerAgg)
	localIPs := map[string]struct{}{"10.0.0.10": {}}
	obs := externalObservation{
		sourceType: "ipfix",
		protocol:   "tcp",
		srcIP:      "10.0.0.10",
		dstIP:      "203.0.113.15",
		srcPort:    51000,
		dstPort:    443,
		bytes:      1000,
		packets:    10,
		firstSeen:  1700000000,
		lastSeen:   1700000010,
	}
	mergeExternalObservation(obs, localIPs, flows, peers)
	if len(flows) != 1 {
		t.Fatalf("expected 1 merged flow, got %d", len(flows))
	}
	if len(peers) != 1 {
		t.Fatalf("expected 1 merged peer, got %d", len(peers))
	}
	for key, flow := range flows {
		if key.process != "ext_ipfix" {
			t.Fatalf("expected process ext_ipfix, got %q", key.process)
		}
		if flow.bytesOut <= 0 {
			t.Fatalf("expected outbound bytes from external flow, got %d", flow.bytesOut)
		}
	}
}

func TestResolveOptionsAppliesCaptureTimeoutBounds(t *testing.T) {
	c := &collector{
		prefs: func() config.CollectPrefs {
			return config.CollectPrefs{
				NetworkCaptureSampleSec: 2,
				NetworkCaptureTimeoutMs: 22000,
			}
		},
		window:       defaultWindow,
		interval:     defaultInterval,
		maxFlows:     defaultMaxFlows,
		maxPeers:     defaultMaxPeers,
		maxListeners: defaultMaxListeners,
	}
	got := c.resolveOptions()
	if got.commandTimeout != maxCmdTimeout {
		t.Fatalf("resolveOptions().commandTimeout = %v, want %v", got.commandTimeout, maxCmdTimeout)
	}

	c.prefs = func() config.CollectPrefs {
		return config.CollectPrefs{
			NetworkCaptureTimeoutMs: 300,
		}
	}
	got = c.resolveOptions()
	if got.commandTimeout != minCmdTimeout {
		t.Fatalf("resolveOptions().commandTimeout = %v, want %v", got.commandTimeout, minCmdTimeout)
	}
}

func TestBuildCaptureSourceScore(t *testing.T) {
	options := passiveCollectOptions{
		requestedMode: "auto",
		pcapEnabled:   false,
	}
	advanced := &passiveAdvancedPayload{
		Sources: []string{"socket_snapshot", "netlink"},
	}
	high := buildCaptureSourceScore(options, advanced, nil)
	if high.Label != "high" {
		t.Fatalf("expected high label, got %q", high.Label)
	}
	if high.Score < 80 {
		t.Fatalf("expected score >= 80, got %d", high.Score)
	}

	advanced.Sources = []string{"socket_snapshot"}
	low := buildCaptureSourceScore(passiveCollectOptions{
		requestedMode: "pcap",
		pcapEnabled:   true,
	}, advanced, []string{"pcap indisponível"})
	if low.Label != "low" {
		t.Fatalf("expected low label, got %q (score=%d)", low.Label, low.Score)
	}
	if low.Score >= 50 {
		t.Fatalf("expected score < 50, got %d", low.Score)
	}
	if low.Reason == "" {
		t.Fatalf("expected non-empty reason")
	}
}

func TestParseExternalSourceAdapterSpec(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantType   string
		wantSource string
		wantTarget string
	}{
		{
			name:       "legacy file with explicit type",
			raw:        "netflow:/tmp/flows.jsonl",
			wantType:   "netflow",
			wantSource: "file",
			wantTarget: "/tmp/flows.jsonl",
		},
		{
			name:       "explicit file adapter",
			raw:        "ipfix:file:/tmp/ipfix.log",
			wantType:   "ipfix",
			wantSource: "file",
			wantTarget: "/tmp/ipfix.log",
		},
		{
			name:       "http fallback with explicit type",
			raw:        "sflow:https://collector.local/sflow",
			wantType:   "sflow",
			wantSource: "http",
			wantTarget: "https://collector.local/sflow",
		},
		{
			name:       "http raw url",
			raw:        "https://collector.local/netflow",
			wantType:   "netflow",
			wantSource: "http",
			wantTarget: "https://collector.local/netflow",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := parseExternalSourceAdapterSpec(tt.raw)
			if spec.SourceType != tt.wantType {
				t.Fatalf("source type = %q, want %q", spec.SourceType, tt.wantType)
			}
			if spec.Adapter != tt.wantSource {
				t.Fatalf("adapter = %q, want %q", spec.Adapter, tt.wantSource)
			}
			if spec.Target != tt.wantTarget {
				t.Fatalf("target = %q, want %q", spec.Target, tt.wantTarget)
			}
		})
	}
}

func TestReadExternalSourceWithAdapterHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"src_ip":"10.0.0.10","dst_ip":"198.51.100.10","src_port":53000,"dst_port":443,"protocol":"tcp","bytes":900,"packets":9},
			{"src_ip":"10.0.0.10","dst_ip":"198.51.100.11","src_port":53001,"dst_port":53,"protocol":"udp","bytes":300,"packets":3}
		]`))
	}))
	defer srv.Close()

	spec := externalSourceSpec{
		SourceType: "netflow",
		Adapter:    "http",
		Target:     srv.URL,
	}
	rows, dropped, err := readExternalSourceWithAdapter(context.Background(), spec, 100, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dropped != 0 {
		t.Fatalf("unexpected dropped count: %d", dropped)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].sourceType != "netflow" {
		t.Fatalf("expected netflow source type, got %q", rows[0].sourceType)
	}
}

func TestReadExternalSourceWithAdapterHTTPNDJSON(t *testing.T) {
	body := strings.Join([]string{
		`{"src_ip":"10.0.0.10","dst_ip":"203.0.113.10","src_port":54000,"dst_port":443,"protocol":"tcp","bytes":1000,"packets":10}`,
		`src_ip=10.0.0.10 dst_ip=203.0.113.11 src_port=54001 dst_port=53 protocol=udp packets=5 bytes=500`,
	}, "\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	spec := externalSourceSpec{
		SourceType: "siem",
		Adapter:    "http",
		Target:     srv.URL,
	}
	rows, dropped, err := readExternalSourceWithAdapter(context.Background(), spec, 100, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dropped != 0 {
		t.Fatalf("unexpected dropped count: %d", dropped)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}
