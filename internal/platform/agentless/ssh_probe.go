package agentless

import (
	"bytes"
	"context"
	"errors"
	"net"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"golang.org/x/crypto/ssh"
)

const (
	sshProbeCommand  = "/usr/bin/cat /proc/loadavg /proc/uptime /proc/meminfo"
	maxSSHProbeReply = 32 << 10
)

var (
	errSSHHostKey      = errors.New("ssh_host_key_mismatch")
	errSSHAccessDenied = errors.New("ssh_access_denied")
	errSSHOutput       = errors.New("invalid_ssh_response")
)

func runSSHProbe(ctx context.Context, job entities.AgentlessJob) Result {
	target, valid := sshProbeEndpoint(job.Endpoint)
	selection, err := monitoring.SSHSelectors(job.Config)
	if !valid || err != nil || job.ProbePolicy == nil {
		return Result{Status: "fail", Code: "invalid_ssh_config"}
	}
	profile, ok := loadSSHProfile(config.AgentlessSSHProfilesFile(), job, selection, target)
	if !ok {
		return Result{Status: "fail", Code: "ssh_credential_unavailable"}
	}
	clientConfig, ok := profile.clientConfig()
	if !ok {
		return Result{Status: "fail", Code: "ssh_credential_unavailable"}
	}

	raw, code, err := readSSHProbe(ctx, target.String(), clientConfig)
	if err != nil {
		return Result{Status: "fail", Code: code, failure: err}
	}
	metrics, err := parseSSHProbe(raw)
	if err != nil {
		return Result{Status: "fail", Code: "invalid_ssh_response", failure: err}
	}
	values := make(map[string]any, len(selection.Metrics))
	for _, metric := range selection.Metrics {
		if value, ok := metrics[metric.Preset]; ok {
			values[selection.Reference(metric)] = value
		}
	}
	return Result{Status: "ok", Payload: map[string]any{"ssh_values_v1": values}}
}

func readSSHProbe(ctx context.Context, address string, config *ssh.ClientConfig) ([]byte, string, error) {
	socket, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, "ssh_connection_failed", err
	}
	defer socket.Close()
	stop := context.AfterFunc(ctx, func() { _ = socket.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		if err := socket.SetDeadline(deadline); err != nil {
			return nil, "ssh_connection_failed", err
		}
	}
	connection, channels, requests, err := ssh.NewClientConn(socket, address, config)
	if err != nil {
		if errors.Is(err, errSSHHostKey) {
			return nil, "ssh_host_key_mismatch", errSSHHostKey
		}
		if sshAuthenticationRejected(err) {
			return nil, errSSHAccessDenied.Error(), errSSHAccessDenied
		}
		return nil, "ssh_connection_failed", err
	}
	client := ssh.NewClient(connection, channels, requests)
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return nil, "ssh_read_failed", err
	}
	closeSocket := func() { _ = socket.Close() }
	out := sshProbeOutput{close: closeSocket}
	stderr := sshProbeOutput{close: closeSocket}
	session.Stdout = &out
	session.Stderr = &stderr
	err = session.Run(sshProbeCommand)
	if out.exceeded || stderr.exceeded {
		return nil, "invalid_ssh_response", errSSHOutput
	}
	if err != nil {
		return nil, "ssh_read_failed", err
	}
	return out.data.Bytes(), "", nil
}

func sshAuthenticationRejected(handshakeErr error) bool {
	// x/crypto wraps exhausted client authentication in a plain error. Pin
	// the two outcomes of our public-key-only configuration at this boundary.
	authErr := errors.Unwrap(handshakeErr)
	if authErr == nil {
		return false
	}
	switch authErr.Error() {
	case "ssh: unable to authenticate, attempted methods [none publickey], no supported methods remain",
		"ssh: unable to authenticate, attempted methods [none], no supported methods remain":
		return true
	default:
		return false
	}
}

type sshProbeOutput struct {
	data     bytes.Buffer
	exceeded bool
	close    func()
}

func (out *sshProbeOutput) Write(data []byte) (int, error) {
	if len(data) > maxSSHProbeReply-out.data.Len() {
		out.exceeded = true
		out.close()
		return 0, errSSHOutput
	}
	return out.data.Write(data)
}
