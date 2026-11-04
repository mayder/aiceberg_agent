//go:build darwin || linux

package outbox

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func BenchmarkBoltStructuredRetainedRead(b *testing.B) {
	rows := strings.Repeat(`{"metric":"cpu","value":17.23,"labels":{"host":"synthetic","source":"relay"}},`, 240)
	body := json.RawMessage(`{"samples":[` + strings.TrimSuffix(rows, ",") + `]}`)
	for _, raw := range []bool{false, true} {
		b.Run(fmt.Sprintf("raw=%v", raw), func(b *testing.B) {
			s, err := NewBoltStoreWithOptions(b.TempDir()+"/synthetic.db", 16, BoltOptions{RawBodies: raw})
			if err != nil {
				b.Fatal(err)
			}
			defer s.Close()
			for i := 0; i < 50; i++ {
				if err := s.Push(entities.Envelope{ID: fmt.Sprint(i), AgentID: "synthetic", Body: body}); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(body) * 50))
			b.ResetTimer()
			cpu := processCPUSeconds()
			for i := 0; i < b.N; i++ {
				frames, err := s.Peek(800)
				if err != nil || len(frames) != 50 {
					b.Fatal("retained read failed", err)
				}
				if _, err := json.Marshal(frames); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			b.ReportMetric((processCPUSeconds()-cpu)*1e6/float64(b.N), "cpu-us/read-forward")
		})
	}
}
