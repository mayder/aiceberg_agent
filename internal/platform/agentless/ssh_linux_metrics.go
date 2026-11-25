package agentless

import (
	"math"
	"strconv"
	"strings"
)

func parseSSHProbe(raw []byte) (map[string]float64, error) {
	lines := strings.Split(string(raw), "\n")
	if len(lines) < 3 {
		return nil, errSSHOutput
	}
	load := strings.Fields(lines[0])
	uptime := strings.Fields(lines[1])
	if len(load) != 5 || len(uptime) != 2 {
		return nil, errSSHOutput
	}
	values := make(map[string]float64, 6)
	for index, name := range []string{"load_1", "load_5", "load_15", "uptime_seconds"} {
		field := uptime[0]
		if index < 3 {
			field = load[index]
		}
		number, err := strconv.ParseFloat(field, 64)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
			return nil, errSSHOutput
		}
		values[name] = number
	}
	seen := make(map[string]bool, 2)
	for _, line := range lines[2:] {
		name, contents, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch name {
		case "MemTotal":
			name = "memory_total_bytes"
		case "MemAvailable":
			name = "memory_available_bytes"
		default:
			continue
		}
		fields := strings.Fields(contents)
		if seen[name] || len(fields) != 2 || fields[1] != "kB" {
			return nil, errSSHOutput
		}
		seen[name] = true
		value, err := strconv.ParseUint(fields[0], 10, 43)
		if err != nil {
			return nil, errSSHOutput
		}
		values[name] = float64(value * 1024)
	}
	if !seen["memory_total_bytes"] {
		return nil, errSSHOutput
	}
	return values, nil
}
