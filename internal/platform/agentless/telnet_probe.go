package agentless

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"github.com/you/aiceberg_agent/internal/platform/telnetprobe"
)

func runTelnetProbe(ctx context.Context, job entities.AgentlessJob) Result {
	selection, err := monitoring.TelnetSelectors(job.Config)
	address, valid := telnetEndpoint(job)
	if err != nil || !valid || job.ProbePolicy == nil {
		return Result{Status: "fail", Code: "invalid_telnet_config"}
	}
	if !telnetTargetAllowed(config.AgentlessTelnetTargetsFile(), job, address) {
		return Result{Status: "fail", Code: "telnet_target_not_allowed"}
	}
	connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", address.String())
	if err != nil {
		return Result{Status: "fail", Code: "telnet_connection_failed", failure: err}
	}
	defer connection.Close()
	reply, err := telnetWorker(ctx, connection.(*net.TCPConn))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Result{Status: "fail", Code: "telnet_read_failed", failure: ctx.Err()}
		}
		return Result{Status: "fail", Code: "telnet_worker_failed", failure: err}
	}
	switch reply {
	case telnetprobe.Banner, telnetprobe.Negotiated:
		value := 0.0
		if reply == telnetprobe.Banner {
			value = 1
		}
		return Result{Status: "ok", Payload: map[string]any{"telnet_values_v1": map[string]any{selection.Reference(): value}}}
	case telnetprobe.Unrecognized:
		return Result{Status: "fail", Code: "telnet_unrecognized_protocol"}
	case telnetprobe.ReplyTooLong:
		return Result{Status: "fail", Code: "telnet_response_limit"}
	default:
		return Result{Status: "fail", Code: "telnet_read_failed"}
	}
}

func telnetWorker(ctx context.Context, connection *net.TCPConn) (string, error) {
	socket, err := connection.File()
	if err != nil {
		return "", err
	}
	defer socket.Close()
	binary, err := os.Executable()
	if err != nil {
		return "", err
	}
	command := exec.CommandContext(ctx, binary, telnetprobe.WorkerArgument)
	command.Env = []string{}
	command.ExtraFiles = []*os.File{socket}
	command.Dir = "/"
	command.WaitDelay = 100 * time.Millisecond
	output := telnetWorkerOutput{}
	command.Stdout = &output
	if err := command.Run(); err != nil {
		return "", err
	}
	reply := output.String()
	if len(reply) < 2 || reply[len(reply)-1] != '\n' {
		return "", errors.New("telnet_worker_invalid_output")
	}
	return reply[:len(reply)-1], nil
}

type telnetWorkerOutput struct {
	data bytes.Buffer
}

func (out *telnetWorkerOutput) String() string { return out.data.String() }

func (out *telnetWorkerOutput) Write(data []byte) (int, error) {
	if len(data) > 32-out.data.Len() {
		return 0, errors.New("telnet_worker_output_limit")
	}
	return out.data.Write(data)
}
