package agentless

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

const postgresStatsQuery = `
SELECT pg_catalog.current_setting('transaction_read_only') = 'on',
       numbackends::float8, xact_commit::float8, xact_rollback::float8,
       EXTRACT(EPOCH FROM (CURRENT_TIMESTAMP - pg_catalog.pg_postmaster_start_time()))::float8
FROM pg_catalog.pg_stat_database
WHERE datname = pg_catalog.current_database()`

func runPostgres(ctx context.Context, job entities.AgentlessJob) Result {
	selection, err := monitoring.PostgresSelectors(job.Config)
	if err != nil {
		return Result{Status: "fail", Code: "invalid_postgresql_config"}
	}
	profile, err := loadPostgresProfile(config.AgentlessPostgresProfilesFile(), job, selection)
	if err != nil {
		return Result{Status: "fail", Code: "postgresql_credential_unavailable"}
	}
	connectionConfig, err := postgresConnectionConfig(profile, job.ProbePolicy.BudgetMS)
	if err != nil {
		return Result{Status: "fail", Code: "invalid_postgresql_config"}
	}
	connection, err := pgx.ConnectConfig(ctx, connectionConfig)
	if err != nil {
		var serverError *pgconn.PgError
		if errors.As(err, &serverError) && (serverError.Code == "28P01" || serverError.Code == "28000") {
			return Result{Status: "fail", Code: "postgresql_access_denied", failure: err}
		}
		return Result{Status: "fail", Code: "postgresql_connection_failed", failure: err}
	}
	defer connection.Close(ctx)
	metrics, err := readPostgresStats(ctx, connection)
	if err != nil {
		var serverError *pgconn.PgError
		code := "postgresql_read_failed"
		if errors.As(err, &serverError) && serverError.Code == "57014" {
			code = "postgresql_statement_timeout"
		}
		return Result{Status: "fail", Code: code, failure: err}
	}
	values := make(map[string]any, len(selection.Metrics))
	for _, metric := range selection.Metrics {
		if value, ok := monitoring.NumericProbeValue(metrics[metric.Preset]); ok {
			values[selection.Reference(metric)] = value
		}
	}
	return Result{Status: "ok", Payload: map[string]any{"postgresql_values_v1": values}}
}

func postgresConnectionConfig(profile postgresProfile, budgetMS int64) (*pgx.ConnConfig, error) {
	// Construct pgx's parsed baseline without loading libpq service files or client certificates.
	if os.Getenv("PGSERVICE") != "" {
		return nil, errPostgresCredential
	}
	cfg, err := pgx.ParseConfig("host=localhost port=5432 dbname=postgres user=monitor password=unused sslmode=disable sslnegotiation=postgres target_session_attrs=any connect_timeout=0")
	if err != nil {
		return nil, errPostgresCredential
	}
	tlsConfig := &tls.Config{ServerName: profile.Host, MinVersion: tls.VersionTLS12}
	if profile.CAPEM != "" {
		cas := x509.NewCertPool()
		if !cas.AppendCertsFromPEM([]byte(profile.CAPEM)) {
			return nil, errPostgresCredential
		}
		tlsConfig.RootCAs = cas
	}
	cfg.Host, cfg.Port = profile.Host, uint16(profile.Port)
	cfg.Database, cfg.User, cfg.Password = profile.Database, profile.User, profile.Password
	cfg.TLSConfig, cfg.Fallbacks, cfg.SSLNegotiation = tlsConfig, nil, "postgres"
	cfg.RequireAuth, cfg.ChannelBinding = "scram-sha-256", "prefer"
	cfg.LookupFunc = probeTCPHostLookup
	cfg.MaxProtocolMessageBodyLen = 128 << 10
	cfg.DefaultQueryExecMode = pgx.QueryExecModeExec
	cfg.RuntimeParams = map[string]string{
		"search_path": "pg_catalog", "application_name": "aiceberg_agentless",
		"default_transaction_read_only": "on", "statement_timeout": strconv.FormatInt(budgetMS, 10),
	}
	return cfg, nil
}

func readPostgresStats(ctx context.Context, connection *pgx.Conn) (map[string]float64, error) {
	tx, err := connection.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var onlyRead bool
	var connections, commits, rollbacks, uptime float64
	err = tx.QueryRow(ctx, postgresStatsQuery).Scan(&onlyRead, &connections, &commits, &rollbacks, &uptime)
	if err != nil {
		return nil, err
	}
	if !onlyRead {
		return nil, errors.New("postgresql_readonly_required")
	}
	if err := tx.Rollback(ctx); err != nil {
		return nil, err
	}
	return map[string]float64{"database_connections": connections, "database_commits": commits,
		"database_rollbacks": rollbacks, "server_uptime_seconds": uptime}, nil
}
