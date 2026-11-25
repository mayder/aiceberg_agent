package sysmetrics

import (
	"testing"
	"time"
)

func TestStoragePassiveStallAndRecovery(t *testing.T) {
	at := time.Unix(1000, 0)
	devices := map[string]bool{"sda": true}
	stat := "cpu 100 0 100 700 100 0 0 0 0 0\nprocs_blocked 9\n"
	psi := "some avg10=98.90 avg60=90 avg300=10 total=1\nfull avg10=97.00 avg60=90 avg300=10 total=1\n"
	prev, _ := parseStorage(stat, psi, "8 0 sda 10 0 0 100 20 0 0 200 98 100 1000", "boot", at, devices)
	now, p := parseStorage("cpu 100 0 100 700 200 0 0 0\nprocs_blocked 12", psi, "8 0 sda 10 0 0 100 20 0 0 200 98 100 981000", "boot", at.Add(10*time.Second), devices)
	storageDelta(prev, now, p)
	if p.PSISome == nil || *p.PSISome != 98.9 || p.Blocked == nil || *p.Blocked != 12 || p.IOWaitPercent == nil || *p.IOWaitPercent != 100 {
		t.Fatalf("missing pressure: %+v", p)
	}
	d := p.Disks[0]
	if d.Stalled == nil || !*d.Stalled || d.AwaitMS != nil || d.Queue == nil || *d.Queue != 98 {
		t.Fatalf("zero completions must not be zero latency: %+v", d)
	}
	recovered, q := parseStorage(stat, psi, "8 0 sda 20 0 0 400 30 0 0 500 0 100 982000", "boot", at.Add(20*time.Second), devices)
	storageDelta(now, recovered, q)
	if q.Disks[0].AwaitMS == nil || *q.Disks[0].AwaitMS != 30 || *q.Disks[0].Stalled {
		t.Fatalf("bad recovery %+v", q.Disks[0])
	}
}

func TestStorageMissingWarmupResetAndStale(t *testing.T) {
	at := time.Unix(1000, 0)
	devices := map[string]bool{"sda": true}
	c, p := parseStorage("", "some avg10=NaN\nfull avg10=Inf", "", "boot", at, devices)
	storageDelta(storageCounters{}, c, p)
	if p.IOWaitPercent != nil || p.PSISome != nil || p.Blocked != nil || len(p.Gaps) < 3 {
		t.Fatal("missing data became healthy zero")
	}
	for _, test := range []struct {
		boot  string
		delay time.Duration
	}{{"new-boot", 10 * time.Second}, {"boot", 121 * time.Second}, {"boot", -time.Second}} {
		old, _ := parseStorage("cpu 10 0 0 100 20 0 0 0", "", "8 0 sda 1 0 0 1 1 0 0 1 2 0 1", "boot", at, devices)
		now, q := parseStorage("cpu 20 0 0 100 30 0 0 0", "", "8 0 sda 1 0 0 1 1 0 0 1 2 0 1", test.boot, at.Add(test.delay), devices)
		storageDelta(old, now, q)
		if q.IOWaitPercent != nil || q.Disks[0].Stalled != nil || q.Disks[0].AwaitMS != nil {
			t.Fatal("invalid interval accepted")
		}
	}
}

func TestStorageRoutineNeverInvokesSMARTByDefault(t *testing.T) {
	t.Setenv("AICEBERG_ALLOW_SMART", "")
	if got := collectOptInSMART(map[string]struct{}{"/dev/does-not-exist": {}}); got != nil {
		t.Fatal("SMART ran without opt in")
	}
}
