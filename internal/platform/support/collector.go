// Package support reads only the agent service's diagnostic sources.
package support

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const MaxBytes = 16 * 1024
const Timeout = 5 * time.Second

type source struct {
	name, command string
	args          []string
}
type runner func(context.Context, source) ([]byte, bool, error)
type Collector struct {
	os  string
	run runner
}

func New() *Collector { return NewForPlatform(runtime.GOOS) }

// NewForPlatform binds a fixed source policy; remote command payloads cannot select it.
func NewForPlatform(platform string) *Collector { return &Collector{os: platform, run: runCommand} }

func (c *Collector) Collect(parent context.Context) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(parent, Timeout)
	defer cancel()
	sources := serviceSources(c.os)
	result := map[string]any{"schema": "aiceberg-agent-support.v1", "os": c.os,
		"max_bytes_per_source": MaxBytes, "timeout_ms": Timeout.Milliseconds()}
	if len(sources) == 0 {
		result["status"] = "unsupported"
		return result, errors.New("support source unsupported")
	}
	rows := make([]any, 0, len(sources))
	available := false
	for _, item := range sources {
		data, truncated, err := c.run(ctx, item)
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		row := map[string]any{"source": item.name, "status": sourceStatus(ctx, err), "truncated": truncated}
		// Never export output from a failed tool, which may contain unfiltered errors.
		if err == nil {
			if len(data) > MaxBytes {
				data = data[:MaxBytes]
				row["truncated"] = true
			}
			text := redact(string(data))
			row["text"] = text
			if strings.TrimSpace(text) == "" || strings.TrimSpace(text) == "-- No entries --" {
				row["status"] = "empty"
			} else {
				available = true
			}
		}
		rows = append(rows, row)
	}
	result["sources"] = rows
	if !available {
		result["status"] = "unavailable"
		return result, errors.New("support evidence unavailable")
	}
	result["status"] = "collected"
	return result, nil
}

func serviceSources(goos string) []source {
	switch goos {
	case "linux":
		return []source{{name: "agent_service_journal", command: "journalctl", args: []string{
			"--unit=aiceberg-agent.service", "--no-pager", "--quiet", "--lines=100", "--output=cat", "--since=-1h"}}}
	case "windows":
		return []source{{name: "agent_service_eventlog", command: "wevtutil", args: []string{
			"qe", "Application", "/q:*[System[Provider[@Name='AIcebergAgent']]]", "/c:100", "/rd:true", "/f:text"}}}
	default:
		return nil
	}
}

func sourceStatus(ctx context.Context, err error) string {
	if err == nil {
		return "collected"
	}
	if ctx.Err() != nil {
		return "cancelled_or_timeout"
	}
	if errors.Is(err, os.ErrPermission) {
		return "permission_denied"
	}
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return "missing"
	}
	return "unavailable"
}

// boundedOutput keeps memory bounded even when the child produces excessive output.
type boundedOutput struct {
	data      []byte
	truncated bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := MaxBytes - len(b.data)
	if n > remaining {
		b.truncated = true
		p = p[:remaining]
	}
	b.data = append(b.data, p...)
	return n, nil
}

func runCommand(ctx context.Context, item source) ([]byte, bool, error) {
	cmd := exec.CommandContext(ctx, item.command, item.args...)
	output := &boundedOutput{}
	cmd.Stdout = output
	stderr := &boundedOutput{}
	cmd.Stderr = stderr
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	diagnostic := strings.ToLower(string(stderr.data))
	if strings.Contains(diagnostic, "permission denied") || strings.Contains(diagnostic, "access is denied") || strings.Contains(diagnostic, "access denied") || strings.Contains(diagnostic, "insufficient permissions") {
		err = os.ErrPermission
	}
	return output.data, output.truncated, err
}
