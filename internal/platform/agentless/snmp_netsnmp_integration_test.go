package agentless

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

const (
	pollingCommunity = "fixture-polling-read"
	pollingAuth      = "fixture-auth-passphrase"
	pollingPriv      = "fixture-priv-passphrase"
	pollingSysName   = "aiceberg-net-snmp-fixture"
	pollingNameOID   = "1.3.6.1.2.1.1.5.0"
	pollingAbsentOID = "1.3.6.1.2.1.1.99.0"
)

func TestSNMPPollingAgainstNetSNMP(t *testing.T) {
	daemon := os.Getenv("AICEBERG_SNMPD_BIN")
	if daemon == "" {
		t.Skip("set AICEBERG_SNMPD_BIN to the Net-SNMP daemon")
	}
	port := startNetSNMPPollingFixture(t, daemon)
	for _, version := range []string{"v1", "v2c", "v3"} {
		t.Run(version, func(t *testing.T) {
			job := netSNMPPollingJob(port, version)
			collect := func(authorized bool) entities.AgentlessObservation {
				partials := 0
				observation := RunJobWithCredentials(context.Background(), job, func(entities.AgentlessObservation) {
					partials++
				}, func(_ context.Context, scoped entities.AgentlessJob) (*entities.AgentlessSnmpProfile, error) {
					return netSNMPPollingCredential(*scoped.SNMP, authorized), nil
				})
				if authorized && partials != 1 || partials+1 > ObservationCost(job) {
					t.Fatalf("unexpected outbox cost: partials=%d reserved=%d", partials, ObservationCost(job))
				}
				return observation
			}
			assertNetSNMPPollingResult(t, collect(true), version)
			rejected := collect(false)
			if rejected.Status == "ok" || rejected.ProbeReport == nil || rejected.ProbeReport.Outcome == "ok" {
				t.Fatal("the wrong credential was accepted")
			}
			assertNetSNMPPollingResult(t, collect(true), version)
		})
	}
}

func netSNMPPollingJob(port int, version string) entities.AgentlessJob {
	job := referenceSNMPJob(port)
	job.SNMP.Version = version
	job.TimeoutMs, job.SNMP.TimeoutMs = 300, 300
	delete(job.Config, "snmp_basic_probe")
	delete(job.Config, "snmp_basic_oid")
	job.Config["groups"] = []string{"system"}
	job.Config["exclude_groups"] = []string{"system"}
	job.Config["snmp_max_rows"] = 3
	job.Config["flush_partial"] = true
	job.Config["custom"] = map[string]any{
		"get":  []any{map[string]any{"oid": pollingAbsentOID}, map[string]any{"oid": pollingNameOID}},
		"walk": []any{map[string]any{"oid": "1.3.6.1.2.1.1"}},
	}
	return job
}

func netSNMPPollingCredential(profile entities.AgentlessSnmpProfile, authorized bool) *entities.AgentlessSnmpProfile {
	if profile.Version == "v3" {
		profile.V3User, profile.V3AuthProtocol, profile.V3PrivProtocol = "fixture-reader", "SHA", "AES"
		profile.V3AuthPassword, profile.V3PrivPassword = pollingAuth, pollingPriv
		if !authorized {
			profile.V3AuthPassword = "fixture-wrong-password"
		}
	} else {
		profile.Community = pollingCommunity
		if !authorized {
			profile.Community = "fixture-wrong-community"
		}
	}
	return &profile
}

func assertNetSNMPPollingResult(t *testing.T, observation entities.AgentlessObservation, version string) {
	t.Helper()
	if observation.Status != "ok" || observation.ProbeReport == nil || observation.ProbeReport.Outcome != "ok" {
		t.Fatalf("polling failed for %s: code=%s report=%+v", version, observation.Code, observation.ProbeReport)
	}
	runtime, _ := observation.Payload["runtime_applied"].(map[string]any)
	custom, _ := observation.Payload["custom"].(map[string]any)
	get, _ := custom["get"].([]any)
	walk, _ := custom["walk"].([]any)
	failed, _ := observation.Payload["oids_failed"].([]any)
	if runtime["snmp_version"] != version || len(get) != 2 || len(walk) != 1 || len(failed) != 1 {
		t.Fatalf("unexpected GET/WALK result for %s: get=%d walk=%d absent=%d", version, len(get), len(walk), len(failed))
	}
	absent, _ := get[0].(map[string]any)
	value, _ := get[1].(map[string]any)
	walkResult, _ := walk[0].(map[string]any)
	if absent["oid"] != pollingAbsentOID || absent["value"] != nil || absent["error"] == nil {
		t.Fatal("an absent OID became a sample")
	}
	if value["oid"] != pollingNameOID || value["value"] != pollingSysName {
		t.Fatal("the sysName returned by Net-SNMP was not preserved")
	}
	if walkResult["rows"] != float64(3) || walkResult["truncated"] != true {
		t.Fatal("the WALK did not enforce the configured row limit")
	}
	wire, err := json.Marshal(observation)
	if err != nil || bytes.Contains(wire, []byte(pollingCommunity)) ||
		bytes.Contains(wire, []byte(pollingAuth)) || bytes.Contains(wire, []byte(pollingPriv)) {
		t.Fatal("the observation could not be encoded safely")
	}
}

func startNetSNMPPollingFixture(t *testing.T, executable string) int {
	t.Helper()
	socket, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := socket.LocalAddr().(*net.UDPAddr).Port
	if err := socket.Close(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, "fixture.conf")
	config := "rocommunity " + pollingCommunity + " 127.0.0.1 .1.3.6.1.2.1.1\n" +
		"createUser fixture-reader SHA " + pollingAuth + " AES " + pollingPriv + "\n" +
		"rouser fixture-reader priv .1.3.6.1.2.1.1\nsysName " + pollingSysName + "\n"
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	daemon := exec.CommandContext(ctx, executable, "-C", "-f", "-Ln", "-p", filepath.Join(dir, "pid"),
		"-c", configPath, "udp:127.0.0.1:"+strconv.Itoa(port))
	daemon.Env = append(os.Environ(), "SNMP_PERSISTENT_DIR="+dir,
		"SNMP_PERSISTENT_FILE="+filepath.Join(dir, "snmpd.conf"), "MIBS=")
	if err := daemon.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = daemon.Wait()
	})
	waitForNetSNMPPollingFixture(t, port)
	assertNetSNMPFixtureAccessControl(t, port)
	return port
}

func assertNetSNMPFixtureAccessControl(t *testing.T, port int) {
	t.Helper()
	client := &gosnmp.GoSNMP{Target: "127.0.0.1", Port: uint16(port), Community: "fixture-wrong-community",
		Version: gosnmp.Version2c, Timeout: 150 * time.Millisecond}
	if err := client.Connect(); err != nil {
		t.Fatal("could not connect to the loopback access-control fixture")
	}
	defer client.Conn.Close()
	packet, err := client.Get([]string{"." + pollingNameOID})
	if err == nil && len(snmpVariables(packet)) != 0 {
		t.Fatal("invalid Net-SNMP fixture: daemon ignored the rocommunity rule; choose a build with access control")
	}
}

func waitForNetSNMPPollingFixture(t *testing.T, port int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		client := &gosnmp.GoSNMP{Target: "127.0.0.1", Port: uint16(port), Community: pollingCommunity,
			Version: gosnmp.Version2c, Timeout: 100 * time.Millisecond, Context: ctx}
		if client.Connect() == nil {
			packet, err := client.Get([]string{"." + pollingNameOID})
			_ = client.Conn.Close()
			if err == nil && len(packet.Variables) == 1 {
				if value, ok := packet.Variables[0].Value.([]byte); ok && string(value) == pollingSysName {
					return
				}
			}
		}
	}
	t.Fatal("the isolated Net-SNMP daemon did not answer on loopback")
}
