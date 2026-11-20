package agentless

import (
	"context"
	"errors"
	"testing"
	"time"

	gosnmp "github.com/gosnmp/gosnmp"
)

func TestSNMPV1AdmissionStopsAtRequestLimitAndCancellation(t *testing.T) {
	job := newSNMPJob()
	job.SNMP.Version = "v1"
	plan := buildSNMPPlan(job, "127.0.0.1")
	plan.SNMPVersion = "v1"
	calls := 0
	collector := snmpCollector{job: job, plan: plan, payload: newSNMPPayload(plan), v1Requests: maxSNMPv1Requests, client: &fakeSNMPClient{getFn: func(_ []string) (*gosnmp.SnmpPacket, error) { calls++; return nil, nil }}}
	if _, err := collector.get([]string{"1.3.6.1.2.1.1.3.0"}); err == nil || calls != 0 || !collector.payload.TimeBudgetExceeded {
		t.Fatal("request cap admitted work")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	collector.v1Requests = 0
	collector.ctx = ctx
	if _, err := collector.get([]string{"1.3.6.1.2.1.1.3.0"}); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal("canceled probe admitted work")
	}
}

func TestSNMPV1AdmissionWaitsForCadenceWithoutIgnoringCancel(t *testing.T) {
	job := newSNMPJob()
	plan := buildSNMPPlan(job, "127.0.0.1")
	ctx, cancel := context.WithCancel(context.Background())
	collector := snmpCollector{ctx: ctx, payload: newSNMPPayload(plan), v1LastGet: time.Now(), started: time.Now(), budget: time.Second}
	cancel()
	if err := collector.admitV1Request(); !errors.Is(err, context.Canceled) {
		t.Fatal("cadence ignored cancellation")
	}
}
