package agentless

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestPostgresProfileRequiresPrivateFileAndExactOwnerAndEndpoint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("private PostgreSQL profiles are provisioned on Unix-like hosts")
	}
	job := postgresTestJob(5432)
	profile := postgresProfile{Reference: "postgres_profile:50", ClientID: 3, AssetID: 52, CheckID: 72, EndpointID: 17,
		Host: "127.0.0.1", Port: 5432, Database: "fixture", User: "fixture_monitor", Password: "fixture-private"}
	path := filepath.Join(t.TempDir(), "profiles.json")
	raw, _ := json.Marshal(map[string]any{"profiles": []postgresProfile{profile}})
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	selection, err := monitoring.PostgresSelectors(job.Config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadPostgresProfile(path, job, selection); err != nil {
		t.Fatal(err)
	}
	otherPort := 15555
	for _, candidate := range []entities.AgentlessJob{
		{ClienteID: 4, AtivoID: 52, CheckID: 72, Endpoint: job.Endpoint, ProbePolicy: job.ProbePolicy},
		{ClienteID: 3, AtivoID: 53, CheckID: 72, Endpoint: job.Endpoint, ProbePolicy: job.ProbePolicy},
		{ClienteID: 3, AtivoID: 52, CheckID: 73, Endpoint: job.Endpoint, ProbePolicy: job.ProbePolicy},
		{ClienteID: 3, AtivoID: 52, CheckID: 72, Endpoint: &entities.AgentlessEndpoint{ID: 17, Tipo: "ip", Endereco: "127.0.0.1", Porta: &otherPort}, ProbePolicy: job.ProbePolicy},
	} {
		if _, err := loadPostgresProfile(path, candidate, selection); err != errPostgresCredential {
			t.Fatal("credential became available for a different scope")
		}
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPostgresProfile(path, job, selection); err != errPostgresCredential {
		t.Fatal("broadly readable credential file accepted")
	}
}

func TestPostgresRejectsRawSQLAndAmbientTLSFallback(t *testing.T) {
	job := postgresTestJob(5432)
	job.Config["sql"] = "SELECT 1"
	obs := RunJob(context.Background(), job)
	if obs.Code != "invalid_postgresql_config" || obs.Message != "" {
		t.Fatal("raw SQL reached the probe")
	}
	t.Setenv("PGSERVICE", "unknown-service")
	profile := postgresProfile{Host: "fixture.local", Port: 5432, User: "fixture", Database: "stats", Password: "fixture-secret"}
	if _, err := postgresConnectionConfig(profile, 1200); err == nil {
		t.Fatal("ambient service configuration accepted")
	}
	t.Setenv("PGSERVICE", "")
	t.Setenv("PGSSLMODE", "allow")
	t.Setenv("PGOPTIONS", "-c default_transaction_read_only=off")
	cfg, err := postgresConnectionConfig(profile, 1200)
	if err != nil || cfg.TLSConfig == nil || cfg.TLSConfig.InsecureSkipVerify || len(cfg.Fallbacks) != 0 ||
		cfg.TLSConfig.ServerName != "fixture.local" || cfg.RuntimeParams["default_transaction_read_only"] != "on" ||
		cfg.RuntimeParams["statement_timeout"] != "1200" || cfg.RuntimeParams["options"] != "" {
		t.Fatal("ambient settings weakened the dedicated connection")
	}
}

func TestPostgresReadsRealDatabaseOverVerifiedTLS(t *testing.T) {
	address := os.Getenv("AICEBERG_POSTGRES_TEST_ADDRESS")
	if address == "" {
		t.Skip("requires a dedicated TLS-enabled PostgreSQL fixture")
	}
	host, portString, err := net.SplitHostPort(address)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() {
		t.Fatal("fixture must be PostgreSQL on loopback")
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatal(err)
	}
	job := postgresTestJob(port)
	job.Endpoint.Endereco = host
	obs := RunJob(context.Background(), job)
	if obs.Status != "ok" || obs.ProbeReport == nil || obs.ProbeReport.Protocol != "postgresql" ||
		obs.MonitoringSamples == nil || len(obs.MonitoringSamples.Samples) != 3 {
		t.Fatalf("TLS PostgreSQL probe failed: %s", obs.Code)
	}
	for i, sample := range obs.MonitoringSamples.Samples {
		if sample.Value == nil || *sample.Value < 0 || (i == 0 && *sample.Value < 1) {
			t.Fatalf("invalid server measurement: %s", sample.Metric)
		}
	}
	wire, _ := json.Marshal(obs)
	if strings.Contains(string(wire), "fixture_monitor") || strings.Contains(string(wire), "fixture-private") {
		t.Fatal("local authentication material escaped into an observation")
	}
	verifyPostgresFixtureRotation(t, job, port, len(obs.MonitoringSamples.Samples))
	verifyPostgresRejectsUntrustedCertificate(t, job)
}

func verifyPostgresFixtureRotation(t *testing.T, job entities.AgentlessJob, port, sampleCount int) {
	t.Helper()
	path := os.Getenv("AICEBERG_POSTGRES_PROFILES_FILE")
	dir := privateSQLFixtureDir(t, path, "aiceberg-pg-")
	if dir == "" {
		return
	}
	selection, err := monitoring.PostgresSelectors(job.Config)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := loadPostgresProfile(path, job, selection)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := pgx.ParseConfig("user=fixture_admin dbname=postgres sslmode=disable")
	if err != nil {
		t.Fatal("configure PostgreSQL fixture rotation")
	}
	cfg.Host, cfg.Port, cfg.Fallbacks = dir, uint16(port), nil
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	admin, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal("open PostgreSQL fixture Unix socket")
	}
	defer admin.Close(ctx)
	if _, err := admin.Exec(ctx, "ALTER ROLE fixture_monitor PASSWORD '"+rotatedSQLFixturePassword+"'"); err != nil {
		t.Fatal("could not rotate PostgreSQL fixture password")
	}
	previousPassword := profile.Password
	profile.Password = rotatedSQLFixturePassword
	verifySQLProbeRotation(t, job, path, profile, "postgresql_access_denied", previousPassword, sampleCount)
}

func verifyPostgresRejectsUntrustedCertificate(t *testing.T, job entities.AgentlessJob) {
	t.Helper()
	selection, err := monitoring.PostgresSelectors(job.Config)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := loadPostgresProfile(os.Getenv("AICEBERG_POSTGRES_PROFILES_FILE"), job, selection)
	if err != nil {
		t.Fatal(err)
	}
	profile.CAPEM = ""
	path := filepath.Join(t.TempDir(), "profiles.json")
	raw, _ := json.Marshal(map[string]any{"profiles": []postgresProfile{profile}})
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PGSSLMODE", "allow")
	t.Setenv("AICEBERG_POSTGRES_PROFILES_FILE", path)
	obs := RunJob(context.Background(), job)
	if obs.Code != "postgresql_connection_failed" || obs.Message != "" || len(obs.Payload) != 0 {
		t.Fatal("invalid certificate prompted a downgrade or exposed the driver error")
	}
}

func postgresTestJob(port int) entities.AgentlessJob {
	return entities.AgentlessJob{ClienteID: 3, AtivoID: 52, CheckID: 72, Tipo: "postgresql", BaseIntervalSec: 60,
		Endpoint: &entities.AgentlessEndpoint{ID: 17, Tipo: "ip", Endereco: "127.0.0.1", Porta: &port},
		ProbePolicy: &monitoring.ProbePolicy{Contract: monitoring.ProbePolicyContract,
			BudgetMS: 5000, FreshnessSec: 180, CredentialRef: "postgres_profile:50"},
		Config: entities.AgentlessConfig{"credential_ref": "postgres_profile:50", "database": "fixture", "metrics": []any{
			map[string]any{"id": "connections", "preset": "database_connections"},
			map[string]any{"id": "rollbacks", "preset": "database_rollbacks"},
			map[string]any{"id": "uptime", "preset": "server_uptime_seconds"},
		}},
	}
}
