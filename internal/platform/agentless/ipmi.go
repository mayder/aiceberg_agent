package agentless

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

const maxIPMIOutput = 256 << 10

var errIPMIOutput = errors.New("invalid_ipmi_response")

func runIPMI(ctx context.Context, job entities.AgentlessJob) Result {
	selection, err := monitoring.IPMISelectors(job.Config)
	if err != nil || job.ProbePolicy == nil {
		return Result{Status: "fail", Code: "invalid_ipmi_config"}
	}
	binary, file := config.AgentlessIPMISettings()
	if !validIPMIBinary(binary) {
		return Result{Status: "fail", Code: "ipmi_tool_unavailable"}
	}
	profile, ok := loadIPMIProfile(file, job, selection)
	if !ok {
		return Result{Status: "fail", Code: "ipmi_credential_unavailable"}
	}
	data, err := readIPMICommand(ctx, binary, profile)
	if err != nil {
		code := "ipmi_read_failed"
		if errors.Is(err, errIPMIOutput) {
			code = "invalid_ipmi_response"
		}
		return Result{Status: "fail", Code: code, failure: err}
	}
	values, err := parseIPMISensors(data, selection)
	if err != nil {
		return Result{Status: "fail", Code: "invalid_ipmi_response"}
	}
	return Result{Status: "ok", Payload: map[string]any{"ipmi_values_v1": values}}
}

func validIPMIBinary(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 && info.Mode().Perm()&0022 == 0
}

type ipmiOutputBuffer struct {
	body     bytes.Buffer
	overflow bool
}

func (b *ipmiOutputBuffer) Write(data []byte) (int, error) {
	if b.body.Len()+len(data) > maxIPMIOutput {
		b.overflow = true
		return 0, errIPMIOutput
	}
	return b.body.Write(data)
}

func readIPMICommand(ctx context.Context, binary string, profile ipmiProfile) ([]byte, error) {
	// Only run the read-only sensor command. Keep credentials out of the argument list.
	command := exec.CommandContext(ctx, binary, "-c", "-I", "lanplus", "-C", "17", "-L", "USER",
		"-N", "1", "-R", "1", "-H", profile.IP, "-p", strconv.Itoa(profile.Port), "-U", profile.Username, "-E", "sensor", "list")
	command.Env = []string{"LC_ALL=C", "IPMI_PASSWORD=" + profile.Password}
	var stdout ipmiOutputBuffer
	command.Stdout, command.Stderr = &stdout, io.Discard
	command.WaitDelay = 250 * time.Millisecond
	err := command.Run()
	if stdout.overflow {
		return nil, errIPMIOutput
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return stdout.body.Bytes(), nil
}

func parseIPMISensors(raw []byte, selection monitoring.IPMISelection) (map[string]any, error) {
	byName := make(map[string][]monitoring.IPMISensor, len(selection.Sensors))
	for _, selector := range selection.Sensors {
		byName[selector.Name] = append(byName[selector.Name], selector)
	}
	values := make(map[string]any, len(selection.Sensors))
	seen := make(map[string]bool, len(selection.Sensors))
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 1024), 4096)
	rows := 0
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ",")
		name := strings.TrimSpace(fields[0])
		if len(fields) == 10 && name != "" {
			rows++
		}
		for _, selector := range byName[name] {
			ref := selection.Reference(selector)
			if seen[ref] {
				delete(values, ref)
				continue
			}
			seen[ref] = true
			if value, ok := ipmiSensorValue(fields, selector.Unit); ok {
				values[ref] = value
			}
		}
	}
	if scanner.Err() != nil || rows == 0 {
		return nil, errIPMIOutput
	}
	return values, nil
}

func ipmiSensorValue(fields []string, unit string) (float64, bool) {
	if len(fields) != 10 || !ipmiReadingStatus(strings.TrimSpace(fields[3])) ||
		!strings.EqualFold(strings.TrimSpace(fields[2]), ipmiWireUnit(unit)) {
		return 0, false
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(fields[1]), 64)
	return value, err == nil && !math.IsInf(value, 0) && !math.IsNaN(value)
}

func ipmiReadingStatus(status string) bool {
	switch strings.ToLower(status) {
	case "ok", "nc", "cr", "nr", "lnc", "lcr", "lnr", "unc", "ucr", "unr":
		return true
	default:
		return false
	}
}

func ipmiWireUnit(unit string) string {
	if unit == "celsius" {
		return "degrees C"
	}
	return unit
}
