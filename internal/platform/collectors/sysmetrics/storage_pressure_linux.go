//go:build linux

package sysmetrics

import (
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type storageSampler struct {
	mu       sync.Mutex
	previous storageCounters
}

func passiveFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 1024*1024))
	if err != nil {
		return ""
	}
	return string(b)
}

func (s *storageSampler) sample() *storageProfile {
	s.mu.Lock()
	defer s.mu.Unlock()
	devices := map[string]bool{}
	entries, _ := os.ReadDir("/sys/block")
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "loop") && !strings.HasPrefix(e.Name(), "ram") {
			devices[e.Name()] = true
		}
	}
	c, p := parseStorage(passiveFile("/proc/stat"), passiveFile("/proc/pressure/io"), passiveFile("/proc/diskstats"), passiveFile("/proc/sys/kernel/random/boot_id"), time.Now(), devices)
	storageDelta(s.previous, c, p)
	s.previous = c
	sort.Slice(p.Disks, func(i, j int) bool { return p.Disks[i].Device < p.Disks[j].Device })
	return p
}
