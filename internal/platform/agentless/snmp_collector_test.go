package agentless

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	gosnmp "github.com/gosnmp/gosnmp"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type fakeSNMPClient struct {
	connectErr error
	getFn      func(oids []string) (*gosnmp.SnmpPacket, error)
	bulkWalkFn func(rootOid string, walkFn gosnmp.WalkFunc) error
	walkFn     func(rootOid string, walkFn gosnmp.WalkFunc) error
}

func (f *fakeSNMPClient) Connect() error {
	return f.connectErr
}

func (f *fakeSNMPClient) Close() error {
	return nil
}

func (f *fakeSNMPClient) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	if f.getFn != nil {
		return f.getFn(oids)
	}
	return &gosnmp.SnmpPacket{}, nil
}

func (f *fakeSNMPClient) BulkWalk(rootOid string, walkFn gosnmp.WalkFunc) error {
	if f.bulkWalkFn != nil {
		return f.bulkWalkFn(rootOid, walkFn)
	}
	return nil
}

func (f *fakeSNMPClient) Walk(rootOid string, walkFn gosnmp.WalkFunc) error {
	if f.walkFn != nil {
		return f.walkFn(rootOid, walkFn)
	}
	return nil
}

func newSNMPJob() entities.AgentlessJob {
	return entities.AgentlessJob{
		Tipo:      "snmp",
		CheckID:   77,
		TimeoutMs: 2000,
		Endpoint: &entities.AgentlessEndpoint{
			Tipo:     "host",
			Endereco: "192.168.0.1",
		},
		SNMP: &entities.AgentlessSnmpProfile{
			ProfileID: 1,
			Version:   "v2c",
			Community: "public",
		},
		Config: entities.AgentlessConfig{
			"collection_profile": "minimal",
			"fetch_mode":         "auto",
		},
	}
}

func withFakeSNMPClient(t *testing.T, c snmpClient) {
	t.Helper()
	prev := defaultSNMPClient
	defaultSNMPClient = func(ctx context.Context, job entities.AgentlessJob, host string) (snmpClient, error) {
		return c, nil
	}
	t.Cleanup(func() {
		defaultSNMPClient = prev
	})
}

func TestRunSNMPTimeBudgetExceeded(t *testing.T) {
	job := newSNMPJob()
	job.Config["fetch_mode"] = "get_only"
	job.Config["time_budget_ms"] = 1

	withFakeSNMPClient(t, &fakeSNMPClient{
		getFn: func(oids []string) (*gosnmp.SnmpPacket, error) {
			time.Sleep(5 * time.Millisecond)
			return nil, errors.New("request timeout no response")
		},
	})

	res := runSNMP(context.Background(), job)
	if res.Status != "fail" {
		t.Fatalf("status esperado fail, obtido %q", res.Status)
	}
	if res.Code != "snmp_time_budget_exceeded" {
		t.Fatalf("code esperado snmp_time_budget_exceeded, obtido %q", res.Code)
	}
	if res.Payload == nil {
		t.Fatalf("payload ausente")
	}
	flag, ok := res.Payload["time_budget_exceeded"].(bool)
	if !ok || !flag {
		t.Fatalf("time_budget_exceeded deveria ser true, payload=%#v", res.Payload["time_budget_exceeded"])
	}
}

func TestRunSNMPMaxRowsPerTable(t *testing.T) {
	job := newSNMPJob()
	job.Config["fetch_mode"] = "walk_only"
	job.Config["snmp_max_rows"] = 3

	withFakeSNMPClient(t, &fakeSNMPClient{
		bulkWalkFn: func(rootOid string, walkFn gosnmp.WalkFunc) error {
			for i := 1; i <= 10; i++ {
				if err := walkFn(gosnmp.SnmpPDU{
					Name:  rootOid + "." + strconv.Itoa(i),
					Type:  gosnmp.Integer,
					Value: i,
				}); err != nil {
					return err
				}
			}
			return nil
		},
	})

	res := runSNMP(context.Background(), job)
	if res.Status != "ok" {
		t.Fatalf("status esperado ok, obtido %q, msg=%q", res.Status, res.Message)
	}
	tables, ok := res.Payload["tables"].(map[string]any)
	if !ok || len(tables) == 0 {
		t.Fatalf("tables vazio no payload: %#v", res.Payload["tables"])
	}
	for oid, rowsRaw := range tables {
		rows, ok := rowsRaw.([]any)
		if !ok {
			t.Fatalf("rows invalidos para %s: %#v", oid, rowsRaw)
		}
		if len(rows) > 3 {
			t.Fatalf("limite snmp_max_rows nao respeitado para %s: %d", oid, len(rows))
		}
	}
}

func TestRunJobSNMPPayloadFields(t *testing.T) {
	job := newSNMPJob()
	job.Config["fetch_mode"] = "get_only"
	job.Config["custom"] = map[string]any{
		"get": []any{"1.3.6.1.2.1.1.3.0"},
	}

	withFakeSNMPClient(t, &fakeSNMPClient{
		getFn: func(oids []string) (*gosnmp.SnmpPacket, error) {
			var vars []gosnmp.SnmpPDU
			for _, oid := range oids {
				vars = append(vars, gosnmp.SnmpPDU{
					Name:  oid,
					Type:  gosnmp.Integer,
					Value: 1,
				})
			}
			return &gosnmp.SnmpPacket{Variables: vars}, nil
		},
	})

	obs := RunJob(context.Background(), job)
	if obs.Status != "ok" {
		t.Fatalf("status esperado ok, obtido %q, msg=%q", obs.Status, obs.Message)
	}
	if obs.Message != "SNMP OK" {
		t.Fatalf("mensagem esperada SNMP OK, obtida %q", obs.Message)
	}
	required := []string{
		"host",
		"profile_id",
		"collection_profile",
		"fetch_mode",
		"time_budget_ms",
		"time_budget_exceeded",
		"groups_requested",
		"stats",
		"group_stats",
		"errors",
		"sys",
		"ifaces",
		"scalars",
		"tables",
		"custom",
	}
	for _, key := range required {
		if _, ok := obs.Payload[key]; !ok {
			t.Fatalf("campo obrigatorio ausente no payload: %s", key)
		}
	}
}
