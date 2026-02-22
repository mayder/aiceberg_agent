package networkcapture

import (
	"testing"
	"time"
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

	got := c.buildSocketSnapshot(flows)
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
