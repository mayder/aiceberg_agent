package telnetprobe

import (
	"bytes"
	"net"
	"testing"
)

func TestInspectTelnetDoesNotTreatTCPTextAsNegotiation(t *testing.T) {
	for _, test := range []struct {
		name     string
		wire     []byte
		want     string
		keepOpen bool
	}{
		{"plaintext", []byte("HTTP/1.1 200 OK\r\n"), Unrecognized, false},
		{"truncated option", []byte{iac, will}, Unrecognized, false},
		{"subnegotiation content", []byte{iac, sb, 1, 'A', iac, iac, 'B', iac, se, iac, wont, 1}, Negotiated, true},
		{"negotiated without banner", []byte{iac, wont, 1}, Negotiated, true},
		{"negotiated with banner", []byte{iac, wont, 1, 'A'}, Banner, true},
		{"oversized reply", bytes.Repeat([]byte{'A'}, ReplyLimit+1), ReplyTooLong, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			connection, server := net.Pipe()
			defer connection.Close()
			defer server.Close()
			go func() {
				_, _ = server.Write(test.wire)
				if !test.keepOpen {
					_ = server.Close()
				}
			}()
			if got := inspect(connection); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}
