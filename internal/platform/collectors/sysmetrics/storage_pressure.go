package sysmetrics

import (
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// Passive kernel accounting only. Never queries a disk/controller management API.
type storageProfile struct {
	SchemaVersion int           `json:"schema_version"`
	WindowSec     float64       `json:"window_sec"`
	IOWaitPercent *float64      `json:"io_wait_percent,omitempty"`
	PSISome       *float64      `json:"psi_io_some_avg10,omitempty"`
	PSIFull       *float64      `json:"psi_io_full_avg10,omitempty"`
	Blocked       *uint64       `json:"blocked_processes,omitempty"`
	Disks         []storageDisk `json:"disks,omitempty"`
	Gaps          []string      `json:"gaps,omitempty"`
}

type storageDisk struct {
	Device   string   `json:"device"`
	InFlight uint64   `json:"in_flight"`
	AwaitMS  *float64 `json:"await_ms,omitempty"`
	Queue    *float64 `json:"avg_queue,omitempty"`
	Stalled  *bool    `json:"no_completions,omitempty"`
}

type storageCounters struct {
	at                time.Time
	boot              string
	cpuTotal, cpuWait uint64
	cpuOK             bool
	disks             map[string][4]uint64 // completed, service ms, weighted ms, in flight
}

func collectOptInSMART(devices map[string]struct{}) []smartState {
	// Direct hardware polling is not part of passive disk monitoring.
	if os.Getenv("AICEBERG_ALLOW_SMART") != "true" {
		return nil
	}
	return collectSMART(devices)
}

func finiteNumber(s string) (float64, bool) {
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil && !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0
}

func parseStorage(stat, psi, disks, boot string, at time.Time, devices map[string]bool) (storageCounters, *storageProfile) {
	c := storageCounters{at: at, boot: strings.TrimSpace(boot), disks: map[string][4]uint64{}}
	p := &storageProfile{SchemaVersion: 1}
	parseStorageCPU(stat, &c, p)
	parseStoragePSI(psi, p)
	parseStorageDisks(disks, devices, &c)
	if p.Blocked == nil {
		p.Gaps = append(p.Gaps, "blocked_processes_unavailable")
	}
	if p.PSISome == nil || p.PSIFull == nil {
		p.Gaps = append(p.Gaps, "psi_io_unavailable")
	}
	if !c.cpuOK {
		p.Gaps = append(p.Gaps, "iowait_unavailable")
	}
	if len(c.disks) == 0 {
		p.Gaps = append(p.Gaps, "disk_accounting_unavailable")
	}
	return c, p
}

func storageDelta(previous, current storageCounters, p *storageProfile) {
	seconds := current.at.Sub(previous.at).Seconds()
	valid := previous.boot != "" && current.boot == previous.boot && seconds >= 1 && seconds <= 120
	if valid {
		p.WindowSec = seconds
	} else {
		p.Gaps = append(p.Gaps, "storage_interval_unavailable")
	}
	if valid && current.cpuOK && previous.cpuOK && current.cpuTotal > previous.cpuTotal && current.cpuWait >= previous.cpuWait {
		v := 100 * float64(current.cpuWait-previous.cpuWait) / float64(current.cpuTotal-previous.cpuTotal)
		if v <= 100 {
			p.IOWaitPercent = &v
		}
	}
	for name, now := range current.disks {
		d := storageDisk{Device: name, InFlight: now[3]}
		old, ok := previous.disks[name]
		if valid && ok && now[0] >= old[0] && now[1] >= old[1] && now[2] >= old[2] {
			completed := now[0] - old[0]
			v := float64(now[2]-old[2]) / (seconds * 1000)
			d.Queue = &v
			stalled := completed == 0 && now[3] > 0 && old[3] > 0
			d.Stalled = &stalled
			if completed > 0 {
				v := float64(now[1]-old[1]) / float64(completed)
				d.AwaitMS = &v
			}
		}
		p.Disks = append(p.Disks, d)
	}
}
