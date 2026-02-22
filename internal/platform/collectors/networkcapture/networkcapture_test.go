package networkcapture

import "testing"

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
