package agentless

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestMySQLProbeReadsTLSFixtureWithNoTableGrants(t *testing.T) {
	profilePath := os.Getenv("AICEBERG_MYSQL_PROBE_PROFILES_FILE")
	address := os.Getenv("AICEBERG_MYSQL_PROBE_FIXTURE_ADDRESS")
	if profilePath == "" || address == "" {
		t.Skip("requires the local fixture from scripts/check-mysql-collector.py")
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		t.Fatal("fixture must be MySQL on loopback")
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	const ref = "mysql_probe_profile:23"
	job := entities.AgentlessJob{
		ClienteID: 3, AtivoID: 52, CheckID: 72, Tipo: "mysql", BaseIntervalSec: 60,
		Endpoint: &entities.AgentlessEndpoint{ID: 17, Tipo: "ip", Endereco: host, Porta: &port},
		Config: entities.AgentlessConfig{"credential_ref": ref, "metrics": []any{
			map[string]any{"id": "connections", "preset": "server_connections"},
			map[string]any{"id": "uptime", "preset": "server_uptime_seconds"},
			map[string]any{"id": "questions", "preset": "server_questions"},
			map[string]any{"id": "aborted", "preset": "server_aborted_connects"},
		}},
		ProbePolicy: &monitoring.ProbePolicy{Contract: monitoring.ProbePolicyContract, BudgetMS: 3000, FreshnessSec: 180, CredentialRef: ref},
	}
	selection, err := monitoring.MySQLSelectors(job.Config)
	if err != nil {
		t.Fatal(err)
	}
	profile, ok := loadMySQLProfile(profilePath, job, selection)
	if !ok {
		t.Fatal("the fixture profile must resolve")
	}
	observation := RunJob(context.Background(), job)
	if os.Getenv("AICEBERG_MYSQL_PROBE_FIXTURE_REFUSES_NATIVE_PASSWORD") == "1" {
		if observation.Code != "mysql_auth_plugin_unsupported" || observation.Status != "fail" ||
			observation.ProbeReport == nil || observation.ProbeReport.Reason != "configuration" || len(observation.Payload) != 0 {
			t.Fatalf("the disallowed authentication did not produce a safe configuration failure: %s", observation.Code)
		}
		return
	}
	if observation.Status != "ok" || observation.ProbeReport == nil || observation.ProbeReport.Protocol != "mysql" ||
		observation.MonitoringSamples == nil || len(observation.MonitoringSamples.Samples) != 4 {
		t.Fatalf("fixture did not return four MySQL status values: %s", observation.Code)
	}
	for i, unit := range []string{"none", "seconds", "none", "none"} {
		got := observation.MonitoringSamples.Samples[i]
		if got.Value == nil || *got.Value < 0 || got.Unit != unit || (i == 0 && *got.Value < 1) {
			t.Fatalf("unexpected MySQL sample: %+v", got)
		}
	}
	wire, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{profile.User, profile.Password, "BEGIN CERTIFICATE", mysqlStatsQuery} {
		if bytes.Contains(wire, []byte(secret)) {
			t.Fatal("private connection detail in observation")
		}
	}
	if dir := privateSQLFixtureDir(t, profilePath, "aiceberg-mysql-probe-"); dir != "" {
		rotateMySQLFixturePassword(t, dir)
		updated := profile
		updated.Password = rotatedSQLFixturePassword
		verifySQLProbeRotation(t, job, profilePath, updated, "mysql_access_denied", profile.Password, len(observation.MonitoringSamples.Samples))
	}
	verifyMySQLProbeRestrictions(t, job, profile)
}

func rotateMySQLFixturePassword(t *testing.T, dir string) {
	t.Helper()
	cfg := mysql.NewConfig()
	cfg.Net, cfg.Addr = "unix", filepath.Join(dir, "mysql.sock")
	cfg.User, cfg.Passwd = "root", "fixture-root-password"
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		t.Fatal("configure MySQL fixture rotation")
	}
	db := sql.OpenDB(connector)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = db.ExecContext(ctx, "ALTER USER 'fixture_mysql_probe'@'127.0.0.1' IDENTIFIED BY '"+rotatedSQLFixturePassword+"'")
	if err != nil {
		t.Fatal("could not rotate MySQL fixture password")
	}
}

func TestMySQLProbeCancelsPendingHandshake(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		socket, err := listener.Accept()
		if err == nil {
			accepted <- socket
		}
	}()
	port := listener.Addr().(*net.TCPAddr).Port
	db, err := openMySQLProbe(mysqlProbeProfile{Host: "127.0.0.1", Port: port, User: "fixture_user"}, 4*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		connection, err := db.Conn(ctx)
		if connection != nil {
			_ = connection.Close()
		}
		result <- err
	}()
	select {
	case socket := <-accepted:
		defer socket.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("probe did not reach the local listener")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled probe kept the handshake open")
	}
}

func verifyMySQLProbeRestrictions(t *testing.T, job entities.AgentlessJob, profile mysqlProbeProfile) {
	t.Helper()
	foreign := job
	foreign.ClienteID++
	if obs := RunJob(context.Background(), foreign); obs.Code != "mysql_credential_unavailable" {
		t.Fatalf("cross-tenant credential accepted: %s", obs.Code)
	}
	custom := job
	custom.Config = entities.AgentlessConfig{
		"credential_ref": profile.Reference,
		"metrics":        job.Config["metrics"],
		"sql":            "SELECT 1",
	}
	if obs := RunJob(context.Background(), custom); obs.Code != "invalid_mysql_config" {
		t.Fatalf("custom SQL was admitted: %s", obs.Code)
	}
	noCA := profile
	noCA.CAPEM = ""
	writeMySQLTestProfile(t, noCA)
	if obs := RunJob(context.Background(), job); obs.Code != "mysql_connection_failed" || len(obs.Payload) != 0 {
		t.Fatalf("untrusted certificate was accepted: %s", obs.Code)
	}
	unauthorized := profile
	unauthorized.Password = "wrong-fixture-password"
	writeMySQLTestProfile(t, unauthorized)
	if obs := RunJob(context.Background(), job); obs.Code != "mysql_access_denied" || len(obs.Payload) != 0 {
		t.Fatalf("wrong password was accepted: %s", obs.Code)
	}
}

func writeMySQLTestProfile(t *testing.T, profile mysqlProbeProfile) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	private, err := json.Marshal(map[string]any{"profiles": []mysqlProbeProfile{profile}})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "mysql-profiles.json")
	if err := os.WriteFile(file, private, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AICEBERG_MYSQL_PROBE_PROFILES_FILE", file)
}
