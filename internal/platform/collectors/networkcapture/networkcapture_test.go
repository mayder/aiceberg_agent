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
