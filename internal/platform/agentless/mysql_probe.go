package agentless

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

const (
	mysqlStatsQuery = "SHOW GLOBAL STATUS WHERE Variable_name IN ('Threads_connected','Uptime','Questions','Aborted_connects')"
	mysqlWireLimit  = 128 << 10
)

var (
	errMySQLCredential = errors.New("mysql_credential_unavailable")
	errMySQLStats      = errors.New("invalid_mysql_response")
	mysqlStatsPresets  = map[string]string{
		"threads_connected": "server_connections",
		"uptime":            "server_uptime_seconds",
		"questions":         "server_questions",
		"aborted_connects":  "server_aborted_connects",
	}
)

func runMySQL(ctx context.Context, job entities.AgentlessJob) Result {
	selection, err := monitoring.MySQLSelectors(job.Config)
	if err != nil || job.ProbePolicy == nil {
		return Result{Status: "fail", Code: "invalid_mysql_config"}
	}
	profile, ok := loadMySQLProfile(config.AgentlessMySQLProfilesFile(), job, selection)
	if !ok {
		return Result{Status: "fail", Code: "mysql_credential_unavailable"}
	}
	db, err := openMySQLProbe(profile, time.Duration(job.ProbePolicy.BudgetMS)*time.Millisecond)
	if err != nil {
		return Result{Status: "fail", Code: "mysql_credential_unavailable"}
	}
	defer db.Close()

	connection, err := db.Conn(ctx)
	if err != nil {
		if errors.Is(err, mysql.ErrNativePassword) {
			return Result{Status: "fail", Code: "mysql_auth_plugin_unsupported", failure: err}
		}
		var serverError *mysql.MySQLError
		if errors.As(err, &serverError) && serverError.Number == 1045 {
			return Result{Status: "fail", Code: "mysql_access_denied", failure: err}
		}
		return Result{Status: "fail", Code: "mysql_connection_failed", failure: err}
	}
	defer connection.Close()

	stats, err := readMySQLProbe(ctx, connection)
	if err != nil {
		code := "mysql_read_failed"
		if errors.Is(err, errMySQLStats) {
			code = "invalid_mysql_response"
		}
		return Result{Status: "fail", Code: code, failure: err}
	}
	values := make(map[string]any, len(selection.Metrics))
	for _, metric := range selection.Metrics {
		if value, ok := stats[metric.Preset]; ok {
			values[selection.Reference(metric)] = value
		}
	}
	return Result{Status: "ok", Payload: map[string]any{"mysql_values_v1": values}}
}

func openMySQLProbe(profile mysqlProbeProfile, budget time.Duration) (*sql.DB, error) {
	trust := &tls.Config{ServerName: profile.Host, MinVersion: tls.VersionTLS12}
	if profile.CAPEM != "" {
		trust.RootCAs = x509.NewCertPool()
		if !trust.RootCAs.AppendCertsFromPEM([]byte(profile.CAPEM)) {
			return nil, errMySQLCredential
		}
	}
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd = profile.User, profile.Password
	cfg.Net, cfg.Addr = "tcp", net.JoinHostPort(profile.Host, strconv.Itoa(profile.Port))
	cfg.TLS = trust
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = budget, budget, budget
	cfg.AllowNativePasswords = false
	cfg.MaxAllowedPacket = mysqlWireLimit
	cfg.Logger = log.New(io.Discard, "", 0)
	cfg.DialFunc = dialMySQLProbe
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	return db, nil
}

func dialMySQLProbe(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := probeTCPHostLookup(ctx, host)
	if err != nil {
		return nil, err
	}
	dialer := net.Dialer{}
	for _, ip := range addresses {
		var connection net.Conn
		connection, err = dialer.DialContext(ctx, network, net.JoinHostPort(ip, port))
		if err == nil {
			return &mysqlWireConn{Conn: connection, reader: io.LimitedReader{R: connection, N: mysqlWireLimit}}, nil
		}
	}
	return nil, err
}

type mysqlWireConn struct {
	net.Conn
	reader io.LimitedReader
}

func (c *mysqlWireConn) Read(buffer []byte) (int, error) {
	return c.reader.Read(buffer)
}

func readMySQLProbe(ctx context.Context, connection *sql.Conn) (map[string]float64, error) {
	tx, err := connection.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	stats, err := readMySQLProbeRows(ctx, tx)
	if err != nil {
		return nil, err
	}
	if err := tx.Rollback(); err != nil {
		return nil, err
	}
	return stats, nil
}

func readMySQLProbeRows(ctx context.Context, tx *sql.Tx) (map[string]float64, error) {
	rows, err := tx.QueryContext(ctx, mysqlStatsQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make(map[string]float64, len(mysqlStatsPresets))
	for rows.Next() {
		var name, raw string
		if err := rows.Scan(&name, &raw); err != nil {
			return nil, errMySQLStats
		}
		preset, known := mysqlStatsPresets[strings.ToLower(name)]
		value, err := strconv.ParseUint(raw, 10, 53)
		if _, repeated := stats[preset]; repeated || !known || err != nil {
			return nil, errMySQLStats
		}
		stats[preset] = float64(value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(stats) == 0 {
		return nil, errMySQLStats
	}
	return stats, nil
}
