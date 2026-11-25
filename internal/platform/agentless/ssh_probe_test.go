package agentless

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"golang.org/x/crypto/ssh"
)

const linuxMetricsFixture = "0.00 0.25 0.50 1/221 99\n41.25 100.00\nMemTotal: 4096 kB\nMemAvailable: 0 kB\n"

func TestSSHProbeReadsLinuxMetricsOverPinnedSSH(t *testing.T) {
	requireSSHProfilePlatform(t)
	job, profile, command := sshFixture(t, linuxMetricsFixture, false)
	observation := RunJob(context.Background(), job)
	if observation.Status != "ok" || observation.ProbeReport == nil || observation.ProbeReport.Protocol != "ssh" ||
		observation.MonitoringSamples == nil || len(observation.MonitoringSamples.Samples) != 3 {
		t.Fatalf("SSH probe did not return three series: %s", observation.Code)
	}
	if got := <-command; got != sshProbeCommand {
		t.Fatalf("unexpected exec request: %q", got)
	}
	wantValues := []float64{0, 41.25, 0}
	wantUnits := []string{"none", "seconds", "bytes"}
	for i, sample := range observation.MonitoringSamples.Samples {
		if sample.Value == nil || *sample.Value != wantValues[i] || sample.Unit != wantUnits[i] {
			t.Fatalf("unexpected sample: %+v", sample)
		}
	}
	wire, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	hostKeyFields := strings.Fields(profile.HostKey)
	privateKeyBlock, _ := pem.Decode([]byte(profile.PrivateKey))
	if bytes.Contains(wire, []byte(hostKeyFields[1])) || bytes.Contains(wire, []byte(privateKeyBlock.Type)) ||
		bytes.Contains(wire, []byte(profile.Username)) || bytes.Contains(wire, []byte("MemAvailable")) {
		t.Fatal("private fixture data in observation")
	}
}

func TestSSHProbeRejectsScopeAndHostMismatch(t *testing.T) {
	requireSSHProfilePlatform(t)
	job, profile, command := sshFixture(t, linuxMetricsFixture, false)
	foreign := job
	foreign.ClienteID++
	if obs := RunJob(context.Background(), foreign); obs.Code != "ssh_credential_unavailable" {
		t.Fatalf("foreign credential was accepted: %s", obs.Code)
	}
	badConfig := job
	badConfig.Config = entities.AgentlessConfig{
		"credential_ref": profile.Reference,
		"metrics":        job.Config["metrics"],
		"command":        "true",
	}
	if obs := RunJob(context.Background(), badConfig); obs.Code != "invalid_ssh_config" {
		t.Fatalf("arbitrary command accepted: %s", obs.Code)
	}
	_, wrongKey := sshFixtureKey(t)
	profile.HostKey = string(ssh.MarshalAuthorizedKey(wrongKey.PublicKey()))
	writeSSHFixtureProfile(t, profile)
	if obs := RunJob(context.Background(), job); obs.Code != "ssh_host_key_mismatch" || len(obs.Payload) != 0 {
		t.Fatalf("wrong host pin accepted: %s", obs.Code)
	}
	select {
	case <-command:
		t.Fatal("exec reached server after key mismatch")
	default:
	}
}

func TestSSHProbeClassifiesAnUnauthorizedKeyAsConfiguration(t *testing.T) {
	requireSSHProfilePlatform(t)
	job, profile, command := sshFixture(t, linuxMetricsFixture, false)
	profile.PrivateKey, _ = sshFixtureKey(t)
	writeSSHFixtureProfile(t, profile)
	result := RunJob(context.Background(), job)
	if result.Status != "fail" || result.Code != "ssh_access_denied" || result.ProbeReport == nil ||
		result.ProbeReport.Reason != "configuration" || len(result.Payload) != 0 {
		t.Fatalf("unauthorized key misclassified: %s %+v", result.Code, result.ProbeReport)
	}
	select {
	case <-command:
		t.Fatal("exec reached server after rejection of the private key")
	default:
	}
}

func TestSSHProbeBoundsReply(t *testing.T) {
	requireSSHProfilePlatform(t)
	job, _, _ := sshFixture(t, strings.Repeat("x", maxSSHProbeReply+1), false)
	if obs := RunJob(context.Background(), job); obs.Code != "invalid_ssh_response" || len(obs.Payload) != 0 {
		t.Fatalf("oversize reply accepted: %s", obs.Code)
	}
}

func TestSSHProbeCancelsOpenSession(t *testing.T) {
	requireSSHProfilePlatform(t)
	job, _, command := sshFixture(t, "", true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan entities.AgentlessObservation, 1)
	go func() { result <- RunJob(ctx, job) }()

	select {
	case <-command:
	case <-time.After(time.Second):
		t.Fatal("SSH command did not reach the fixture")
	}
	cancel()
	select {
	case obs := <-result:
		if obs.Status != "fail" || obs.ProbeReport == nil || obs.ProbeReport.Outcome != "cancelled" {
			t.Fatalf("unexpected canceled probe report: %+v", obs.ProbeReport)
		}
	case <-time.After(time.Second):
		t.Fatal("SSH cancellation did not close the socket")
	}
}

func TestParseSSHProbeOlderKernelAndInvalidReplies(t *testing.T) {
	values, err := parseSSHProbe([]byte(strings.ReplaceAll(linuxMetricsFixture, "MemAvailable: 0 kB\n", "")))
	if err != nil || values["memory_total_bytes"] != 4194304 {
		t.Fatalf("expected valid older kernel fixture: %v %v", values, err)
	}
	if _, ok := values["memory_available_bytes"]; ok {
		t.Fatal("missing memory value became a measurement")
	}
	for _, raw := range []string{
		strings.Replace(linuxMetricsFixture, "0.00", "NaN", 1),
		strings.Replace(linuxMetricsFixture, "4096 kB", "-1 kB", 1),
		linuxMetricsFixture + "MemTotal: 4096 kB\n",
		strings.Replace(linuxMetricsFixture, "4096 kB", "4096 MB", 1),
	} {
		if _, err := parseSSHProbe([]byte(raw)); err == nil {
			t.Fatal("invalid SSH reply accepted")
		}
	}
}

func requireSSHProfilePlatform(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("private credential resolution requires a supported Unix platform")
	}
}

func sshFixture(t *testing.T, reply string, wait bool) (entities.AgentlessJob, sshProbeProfile, <-chan string) {
	t.Helper()
	_, hostKey := sshFixtureKey(t)
	privateKey, clientKey := sshFixtureKey(t)
	const user = "linux_fixture_reader"
	serverConfig := &ssh.ServerConfig{
		PublicKeyCallback: func(metadata ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if metadata.User() != user || !bytes.Equal(key.Marshal(), clientKey.PublicKey().Marshal()) {
				return nil, ssh.ErrNoAuth
			}
			return nil, nil
		},
	}
	serverConfig.AddHostKey(hostKey)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	command := make(chan string, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		socket, err := listener.Accept()
		if err == nil {
			defer socket.Close()
			_ = socket.SetDeadline(time.Now().Add(5 * time.Second))
			serveSSHFixture(socket, serverConfig, command, reply, wait)
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-done
	})
	port := listener.Addr().(*net.TCPAddr).Port
	const ref = "ssh_probe_profile:15"
	profile := sshProbeProfile{
		Reference: ref, ClientID: 1, AssetID: 2, CheckID: 3, EndpointID: 4,
		IP: "127.0.0.1", Port: port, Username: user,
		HostKey: string(ssh.MarshalAuthorizedKey(hostKey.PublicKey())), PrivateKey: privateKey,
	}
	writeSSHFixtureProfile(t, profile)
	return entities.AgentlessJob{
		ClienteID: 1, AtivoID: 2, CheckID: 3, Tipo: "ssh", BaseIntervalSec: 60,
		Endpoint: &entities.AgentlessEndpoint{ID: 4, Tipo: "ip", Endereco: profile.IP, Porta: &port},
		Config: entities.AgentlessConfig{"credential_ref": ref, "metrics": []any{
			map[string]any{"id": "load", "preset": "load_1"},
			map[string]any{"id": "uptime", "preset": "uptime_seconds"},
			map[string]any{"id": "mem", "preset": "memory_available_bytes"},
		}},
		ProbePolicy: &monitoring.ProbePolicy{Contract: monitoring.ProbePolicyContract, BudgetMS: 3000, FreshnessSec: 180, CredentialRef: ref},
	}, profile, command
}

func serveSSHFixture(socket net.Conn, config *ssh.ServerConfig, command chan<- string, reply string, wait bool) {
	conn, channels, requests, err := ssh.NewServerConn(socket, config)
	if err != nil {
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(requests)
	next, ok := <-channels
	if !ok {
		return
	}
	if next.ChannelType() != "session" {
		_ = next.Reject(ssh.UnknownChannelType, "fixture requires a single session")
		return
	}
	session, requests, err := next.Accept()
	if err != nil {
		return
	}
	defer session.Close()
	req, ok := <-requests
	var exec struct{ Command string }
	if !ok || req.Type != "exec" || ssh.Unmarshal(req.Payload, &exec) != nil {
		return
	}
	command <- exec.Command
	_ = req.Reply(true, nil)
	if wait {
		_ = conn.Wait()
		return
	}
	_, _ = io.WriteString(session, reply)
	_, _ = session.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
}

func sshFixtureKey(t *testing.T) (string, ssh.Signer) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(key, "")
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(block)), signer
}

func writeSSHFixtureProfile(t *testing.T, profile sshProbeProfile) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"profiles": []sshProbeProfile{profile}})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "profiles.json")
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AICEBERG_SSH_PROBE_PROFILES_FILE", file)
}
