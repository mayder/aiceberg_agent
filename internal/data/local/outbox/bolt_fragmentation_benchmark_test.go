//go:build darwin || linux

package outbox

import (
	"fmt"
	"strings"
	"syscall"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func processCPUSeconds() float64 {
	var r syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &r)
	return float64(r.Utime.Sec+r.Stime.Sec) + float64(r.Utime.Usec+r.Stime.Usec)/1e6
}

// Sequential arrivals intentionally cannot coalesce in a Batch. The retained
// synthetic backlog has already drained, leaving reusable pages in the file.
func BenchmarkBoltFragmentedTrickle(b *testing.B) {
	for _, noFree := range []bool{false, true} {
		b.Run(fmt.Sprintf("rebuild=%v", noFree), func(b *testing.B) {
			s := fragmentedBoltFixture(b, true)
			s.db.NoFreelistSync = noFree
			body := strings.Repeat("x", 16384)
			before := s.db.Stats()
			cpu := processCPUSeconds()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				id := fmt.Sprintf("arrival-%08d", i)
				if err := s.Push(entities.Envelope{ID: id, Body: body}); err != nil {
					b.Fatal(err)
				}
				if _, err := s.Peek(800); err != nil {
					b.Fatal(err)
				}
				if err := s.Delete([]string{id}); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			cpu = processCPUSeconds() - cpu
			after := s.db.Stats()
			delta := after.Sub(&before)
			b.ReportMetric(cpu*1e6/float64(b.N), "cpu-us/arrival")
			b.ReportMetric(float64(delta.TxStats.PageAlloc)/float64(b.N), "page-allocated-B/arrival")
			b.ReportMetric(float64(delta.TxStats.Write)/float64(b.N), "writes/arrival")
			if n, _ := s.Len(); n != 64 {
				b.Fatalf("retained fixture changed: %d", n)
			}
		})
	}
}
