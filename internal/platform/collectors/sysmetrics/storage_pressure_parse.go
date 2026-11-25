package sysmetrics

import (
	"strconv"
	"strings"
)

func parseStorageCPU(stat string, c *storageCounters, p *storageProfile) {
	for _, line := range strings.Split(stat, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == "procs_blocked" {
			if v, e := strconv.ParseUint(f[1], 10, 64); e == nil {
				p.Blocked = &v
			}
		}
		if len(f) < 9 || f[0] != "cpu" {
			continue
		}
		c.cpuOK = true
		for i := 1; i <= 8; i++ {
			v, e := strconv.ParseUint(f[i], 10, 64)
			if e != nil {
				c.cpuOK = false
				break
			}
			c.cpuTotal += v
			if i == 5 {
				c.cpuWait = v
			}
		}
	}
}

func parseStoragePSI(psi string, p *storageProfile) {
	for _, line := range strings.Split(psi, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		for _, field := range f[1:] {
			if !strings.HasPrefix(field, "avg10=") {
				continue
			}
			v, ok := finiteNumber(strings.TrimPrefix(field, "avg10="))
			if !ok || v > 100 {
				continue
			}
			if f[0] == "some" {
				p.PSISome = &v
			}
			if f[0] == "full" {
				p.PSIFull = &v
			}
		}
	}
}

func parseStorageDisks(disks string, devices map[string]bool, c *storageCounters) {
	for _, line := range strings.Split(disks, "\n") {
		f := strings.Fields(line)
		if len(f) < 14 || !devices[f[2]] {
			continue
		}
		var values [7]uint64
		valid := true
		for i, index := range []int{3, 7, 6, 10, 13, 11, 12} {
			v, e := strconv.ParseUint(f[index], 10, 64)
			if e != nil {
				valid = false
				break
			}
			values[i] = v
		}
		if valid {
			// Discard and flush completions also advance the queue on modern kernels.
			for _, indexes := range [][2]int{{14, 17}, {18, 19}} {
				if len(f) <= indexes[1] {
					continue
				}
				count, e1 := strconv.ParseUint(f[indexes[0]], 10, 64)
				elapsed, e2 := strconv.ParseUint(f[indexes[1]], 10, 64)
				if e1 != nil || e2 != nil {
					valid = false
					break
				}
				values[0] += count
				values[2] += elapsed
			}
		}
		if valid {
			c.disks[f[2]] = [4]uint64{values[0] + values[1], values[2] + values[3], values[4], values[5]}
		}
	}
}
