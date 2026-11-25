package agentless

import (
	"context"
	"net"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func canonicalTCPJob(port int) entities.AgentlessJob {
	return entities.AgentlessJob{ClienteID: 3, AtivoID: 52, CheckID: 73, Tipo: "tcp", IntervalSec: 60, TimeoutMs: 1000,
		Endpoint: &entities.AgentlessEndpoint{ID: 8, Endereco: "127.0.0.1", Porta: &port}}
}

func TestTCPMonitoringUsesOneExistingConnection(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	done := make(chan struct{})
	acceptedConnection := make(chan struct{}, 1)
	go func() {
		defer close(done)
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			calls.Add(1)
			select {
			case acceptedConnection <- struct{}{}:
			default:
			}
			c.Close()
		}
	}()
	port := listener.Addr().(*net.TCPAddr).Port
	job := canonicalTCPJob(port)
	o := RunJob(context.Background(), job)
	select {
	case <-acceptedConnection:
	case <-time.After(2 * time.Second):
		listener.Close()
		<-done
		t.Fatal("TCP fixture did not accept connection")
	}
	listener.Close()
	<-done
	if o.Status != "ok" || o.MonitoringSamples == nil || calls.Load() != 1 {
		t.Fatalf("TCP collected more than once or absent metadata: %+v calls=%d", o, calls.Load())
	}
	sample := o.MonitoringSamples.Samples[0]
	if sample.State != "ok" || sample.Value == nil || *sample.Value != float64(o.LatencyMs) || sample.Unit != "milliseconds" {
		t.Fatal("latency mismatch", sample)
	}
	qualifyTCPMonitoringGraph(t, o)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	failed := RunJob(ctx, job)
	if failed.Status != "fail" || failed.MonitoringSamples == nil || failed.MonitoringSamples.Samples[0].Gap != "observation_failed" || failed.MonitoringSamples.Samples[0].Value != nil {
		t.Fatal("canceled TCP attempt became a valid latency", failed.Status)
	}
	raw := newObservationFromResult(job, Result{Status: "ok", LatencyMs: 37}, time.Now())
	raw.ObservedAt = time.Unix(1800000000, 0)
	want := raw
	got := withMonitoringSamples(job, raw)
	got.MonitoringSamples = nil
	if !reflect.DeepEqual(got, want) {
		t.Fatal("legacy observation changed")
	}
	raw.IsPartial = true
	if s := withMonitoringSamples(job, raw).MonitoringSamples; s == nil || s.Samples[0].Gap != "partial" {
		t.Fatal("TCP partial promoted")
	}
	raw.IsPartial = false
	raw.SegmentSeq = 1
	raw.IsFinal = false
	if s := withMonitoringSamples(job, raw).MonitoringSamples; s == nil || s.Samples[0].Gap != "partial" {
		t.Fatal("unfinished segment promoted")
	}
	raw.SegmentSeq = 0
	job.Config = entities.AgentlessConfig{"diagnostic_probe": true}
	if withMonitoringSamples(job, raw).MonitoringSamples != nil {
		t.Fatal("diagnostic projected")
	}
	job.Config = nil
	raw.CheckID = 74
	if withMonitoringSamples(job, raw).MonitoringSamples != nil {
		t.Fatal("foreign check projected")
	}
	raw.CheckID = 73
	raw.ObservedAt = time.Time{}
	if withMonitoringSamples(job, raw).MonitoringSamples != nil {
		t.Fatal("invalid clock emitted")
	}
}
