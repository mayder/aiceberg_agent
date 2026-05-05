package agentless

import (
	"context"
	"errors"
	"slices"
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
		"collection_kind",
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

func TestRunJobSNMPCustomVendorOIDProfileColetaExatamenteOIDsEnviados(t *testing.T) {
	job := newSNMPJob()
	job.Config["collection_profile"] = "minimal"
	job.Config["fetch_mode"] = "get_only"
	job.Config["vendor_oid_profile"] = map[string]any{
		"profile_key":     "ale_oaw_ap_base_v1",
		"profile_version": "1.0.0",
		"source_mib":      "OAW-APxxxx",
		"applied":         true,
	}
	job.Config["custom_get_oids"] = []any{
		map[string]any{
			"name":          "ap_cpu_utilization",
			"oid":           "1.3.6.1.4.1.6486.802.1.1.2.1.2.5.1.1.8.0",
			"label":         "CPU do AP",
			"metric":        "apCpuUtilization",
			"source_mib":    "OAW-APxxxx",
			"unit":          "percent",
			"canonical_key": "cpu_percent",
		},
		map[string]any{
			"name":          "ap_memory_utilization",
			"oid":           "1.3.6.1.4.1.6486.802.1.1.2.1.2.5.1.1.7.0",
			"metric":        "apMemoryUtilization",
			"source_mib":    "OAW-APxxxx",
			"unit":          "percent",
			"canonical_key": "memory_percent",
		},
	}
	want := []string{
		"1.3.6.1.4.1.6486.802.1.1.2.1.2.5.1.1.8.0",
		"1.3.6.1.4.1.6486.802.1.1.2.1.2.5.1.1.7.0",
	}
	customGetSeen := false

	withFakeSNMPClient(t, &fakeSNMPClient{
		getFn: func(oids []string) (*gosnmp.SnmpPacket, error) {
			if !slices.Equal(oids, want) {
				var vars []gosnmp.SnmpPDU
				for _, oid := range oids {
					vars = append(vars, gosnmp.SnmpPDU{Name: oid, Type: gosnmp.Integer, Value: 1})
				}
				return &gosnmp.SnmpPacket{Variables: vars}, nil
			}
			customGetSeen = true
			return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{
				{Name: want[0], Type: gosnmp.Integer, Value: 12},
			}}, nil
		},
	})

	obs := RunJob(context.Background(), job)
	if obs.Status != "ok" {
		t.Fatalf("status esperado ok, obtido %q, msg=%q", obs.Status, obs.Message)
	}
	if !customGetSeen {
		t.Fatalf("custom GET nao executado")
	}
	custom, ok := obs.Payload["custom"].(map[string]any)
	if !ok {
		t.Fatalf("custom ausente: %#v", obs.Payload["custom"])
	}
	getItems, ok := custom["get"].([]any)
	if !ok || len(getItems) != 2 {
		t.Fatalf("custom.get inesperado: %#v", custom["get"])
	}
	first, ok := getItems[0].(map[string]any)
	if !ok {
		t.Fatalf("custom.get[0] inesperado: %#v", getItems[0])
	}
	if first["oid"] != want[0] || first["name"] != "ap_cpu_utilization" || first["label"] != "CPU do AP" || first["metric"] != "apCpuUtilization" || first["source_mib"] != "OAW-APxxxx" || first["unit"] != "percent" || first["canonical_key"] != "cpu_percent" {
		t.Fatalf("metadados custom.get perdidos: %#v", first)
	}
	second, ok := getItems[1].(map[string]any)
	if !ok || second["oid"] != want[1] || second["error"] != "sem retorno" {
		t.Fatalf("custom.get falho inesperado: %#v", getItems[1])
	}
	vendorProfile, ok := obs.Payload["vendor_oid_profile"].(map[string]any)
	if !ok || vendorProfile["profile_key"] != "ale_oaw_ap_base_v1" {
		t.Fatalf("vendor_oid_profile inesperado: %#v", obs.Payload["vendor_oid_profile"])
	}
	if obs.Payload["vendor_profile_applied"] != true {
		t.Fatalf("vendor_profile_applied deveria ser true: %#v", obs.Payload["vendor_profile_applied"])
	}
	if obs.Payload["profile_key"] != "ale_oaw_ap_base_v1" || obs.Payload["profile_version"] != "1.0.0" {
		t.Fatalf("profile metadata inesperado: key=%#v version=%#v", obs.Payload["profile_key"], obs.Payload["profile_version"])
	}
	if obs.Payload["fallback_used"] != true {
		t.Fatalf("fallback_used deveria ser true: %#v", obs.Payload["fallback_used"])
	}
	success, ok := obs.Payload["oids_success"].([]any)
	if !ok || len(success) != 1 {
		t.Fatalf("oids_success inesperado: %#v", obs.Payload["oids_success"])
	}
	failed, ok := obs.Payload["oids_failed"].([]any)
	if !ok || len(failed) != 1 {
		t.Fatalf("oids_failed inesperado: %#v", obs.Payload["oids_failed"])
	}
	failedOID, ok := failed[0].(map[string]any)
	if !ok || failedOID["oid"] != want[1] || failedOID["error"] != "sem retorno" || failedOID["canonical_key"] != "memory_percent" {
		t.Fatalf("erro por OID inesperado: %#v", failed[0])
	}
}

func TestRunJobSNMPCustomWalkRetornaSucessoEFalhaPorOID(t *testing.T) {
	job := newSNMPJob()
	job.Config["collection_profile"] = "minimal"
	job.Config["fetch_mode"] = "walk_only"
	job.Config["vendor_oid_profile"] = map[string]any{
		"profile_key":     "ale_omniswitch_aos_health_v1",
		"profile_version": "1.0.0",
		"source_mib":      "ALCATEL-IND1-HEALTH-MIB",
	}
	okOID := "1.3.6.1.4.1.6486.801.1.2.1.16.1.1.1.1.1.15"
	failOID := "1.3.6.1.4.1.6486.801.1.2.1.16.1.1.1.1.1.16"
	job.Config["custom_walk_oids"] = []any{
		map[string]any{
			"name":          "health_module_cpu_latest",
			"oid":           okOID,
			"metric":        "healthModuleCpuLatest",
			"source_mib":    "ALCATEL-IND1-HEALTH-MIB",
			"unit":          "percent",
			"canonical_key": "cpu_percent",
		},
		map[string]any{
			"name":          "health_module_memory_latest",
			"oid":           failOID,
			"metric":        "healthModuleMemoryLatest",
			"source_mib":    "ALCATEL-IND1-HEALTH-MIB",
			"unit":          "percent",
			"canonical_key": "memory_percent",
		},
	}
	customWalkSeen := map[string]bool{}

	withFakeSNMPClient(t, &fakeSNMPClient{
		bulkWalkFn: func(rootOid string, walkFn gosnmp.WalkFunc) error {
			switch rootOid {
			case okOID:
				customWalkSeen[rootOid] = true
				return walkFn(gosnmp.SnmpPDU{Name: rootOid + ".1", Type: gosnmp.Integer, Value: 45})
			case failOID:
				customWalkSeen[rootOid] = true
				return errors.New("no such object")
			default:
				return nil
			}
		},
		walkFn: func(rootOid string, walkFn gosnmp.WalkFunc) error {
			if rootOid == failOID {
				return errors.New("no such object")
			}
			return nil
		},
	})

	obs := RunJob(context.Background(), job)
	if obs.Status != "ok" {
		t.Fatalf("status esperado ok, obtido %q, msg=%q", obs.Status, obs.Message)
	}
	if !customWalkSeen[okOID] || !customWalkSeen[failOID] {
		t.Fatalf("custom walk nao executou ambos OIDs: %#v", customWalkSeen)
	}
	success, ok := obs.Payload["oids_success"].([]any)
	if !ok || len(success) != 1 {
		t.Fatalf("oids_success inesperado: %#v", obs.Payload["oids_success"])
	}
	failed, ok := obs.Payload["oids_failed"].([]any)
	if !ok || len(failed) != 1 {
		t.Fatalf("oids_failed inesperado: %#v", obs.Payload["oids_failed"])
	}
	failedOID, ok := failed[0].(map[string]any)
	if !ok || failedOID["oid"] != failOID || failedOID["error"] != "no such object" {
		t.Fatalf("erro custom walk inesperado: %#v", failed[0])
	}
	if obs.Payload["vendor_profile_applied"] != true {
		t.Fatalf("vendor_profile_applied deveria ser true: %#v", obs.Payload["vendor_profile_applied"])
	}
}

func TestRunJobSNMPCanonicalizaOAWCpuMemoria(t *testing.T) {
	job := newSNMPJob()
	job.Config["collection_profile"] = "minimal"
	job.Config["fetch_mode"] = "get_only"
	job.Config["vendor_oid_profile"] = map[string]any{
		"profile_key":     "ale_oaw_ap_base_v1",
		"profile_version": "1.0.0",
		"source_mib":      "OAW-AP1232",
	}
	cpuOID := "1.3.6.1.4.1.6486.802.1.1.2.1.2.5.1.1.8.0"
	memOID := "1.3.6.1.4.1.6486.802.1.1.2.1.2.5.1.1.7.0"
	job.Config["custom_get_oids"] = []any{
		map[string]any{
			"name":          "ap_cpu_utilization",
			"oid":           cpuOID,
			"metric":        "apCpuUtilization",
			"source_mib":    "OAW-AP1232",
			"unit":          "percent",
			"canonical_key": "cpu_percent",
		},
		map[string]any{
			"name":          "ap_memory_utilization",
			"oid":           memOID,
			"metric":        "apMemoryUtilization",
			"source_mib":    "OAW-AP1232",
			"unit":          "percent",
			"canonical_key": "memory_percent",
		},
	}

	withFakeSNMPClient(t, &fakeSNMPClient{
		getFn: func(oids []string) (*gosnmp.SnmpPacket, error) {
			if slices.Equal(oids, []string{cpuOID, memOID}) {
				return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{
					{Name: cpuOID, Type: gosnmp.Integer, Value: 17},
					{Name: memOID, Type: gosnmp.Integer, Value: 62},
				}}, nil
			}
			return &gosnmp.SnmpPacket{}, nil
		},
	})

	obs := RunJob(context.Background(), job)
	if obs.Status != "ok" {
		t.Fatalf("status esperado ok, obtido %q, msg=%q", obs.Status, obs.Message)
	}
	if obs.Payload["cpu_percent"] != float64(17) || obs.Payload["memory_percent"] != float64(62) {
		t.Fatalf("cpu/mem canonicos inesperados: cpu=%#v mem=%#v", obs.Payload["cpu_percent"], obs.Payload["memory_percent"])
	}
	cpuMemory, ok := obs.Payload["cpu_memory"].(map[string]any)
	if !ok {
		t.Fatalf("cpu_memory ausente: %#v", obs.Payload["cpu_memory"])
	}
	if cpuMemory["source_profile"] != "ale_oaw_ap_base_v1" {
		t.Fatalf("source_profile inesperado: %#v", cpuMemory["source_profile"])
	}
	samples, ok := cpuMemory["vendor_samples"].([]any)
	if !ok || len(samples) != 2 {
		t.Fatalf("vendor_samples inesperado: %#v", cpuMemory["vendor_samples"])
	}
	firstSample, ok := samples[0].(map[string]any)
	if !ok || firstSample["oid"] != cpuOID || firstSample["canonical_key"] != "cpu_percent" || firstSample["source_mib"] != "OAW-AP1232" || firstSample["value"] != float64(17) {
		t.Fatalf("vendor_sample de CPU inesperado: %#v", samples[0])
	}
	success, ok := obs.Payload["oids_success"].([]any)
	if !ok || len(success) != 2 {
		t.Fatalf("oids_success canonico inesperado: %#v", obs.Payload["oids_success"])
	}
	successCPU, ok := success[0].(map[string]any)
	if !ok || successCPU["oid"] != cpuOID || successCPU["canonical_key"] != "cpu_percent" || successCPU["source_mib"] != "OAW-AP1232" {
		t.Fatalf("metadados de sucesso por OID inesperados: %#v", success[0])
	}
	if _, ok := obs.Payload["custom"].(map[string]any); !ok {
		t.Fatalf("custom bruto deveria permanecer no payload")
	}
	if _, ok := obs.Payload["oids"].(map[string]any); !ok {
		t.Fatalf("oids bruto deveria permanecer no payload")
	}
	if _, ok := obs.Payload["scalars"].(map[string]any); !ok {
		t.Fatalf("scalars bruto deveria permanecer no payload")
	}
}

func TestRunJobSNMPCanonicalizaOmniSwitchWalkCpuMemoria(t *testing.T) {
	job := newSNMPJob()
	job.Config["collection_profile"] = "minimal"
	job.Config["fetch_mode"] = "walk_only"
	job.Config["vendor_oid_profile"] = map[string]any{
		"profile_key":     "ale_omniswitch_aos_health_v1",
		"profile_version": "1.0.0",
		"source_mib":      "ALCATEL-IND1-HEALTH-MIB",
	}
	cpuOID := "1.3.6.1.4.1.6486.801.1.2.1.16.1.1.1.1.1.15"
	memOID := "1.3.6.1.4.1.6486.801.1.2.1.16.1.1.1.1.1.16"
	job.Config["custom_walk_oids"] = []any{
		map[string]any{
			"name":          "health_module_cpu_latest",
			"oid":           cpuOID,
			"metric":        "healthModuleCpuLatest",
			"source_mib":    "ALCATEL-IND1-HEALTH-MIB",
			"unit":          "percent",
			"canonical_key": "cpu_percent",
		},
		map[string]any{
			"name":          "health_module_memory_latest",
			"oid":           memOID,
			"metric":        "healthModuleMemoryLatest",
			"source_mib":    "ALCATEL-IND1-HEALTH-MIB",
			"unit":          "percent",
			"canonical_key": "memory_percent",
		},
	}

	withFakeSNMPClient(t, &fakeSNMPClient{
		bulkWalkFn: func(rootOid string, walkFn gosnmp.WalkFunc) error {
			switch rootOid {
			case cpuOID:
				return walkFn(gosnmp.SnmpPDU{Name: rootOid + ".1", Type: gosnmp.Integer, Value: 41})
			case memOID:
				return walkFn(gosnmp.SnmpPDU{Name: rootOid + ".1", Type: gosnmp.Integer, Value: 73})
			default:
				return nil
			}
		},
	})

	obs := RunJob(context.Background(), job)
	if obs.Status != "ok" {
		t.Fatalf("status esperado ok, obtido %q, msg=%q", obs.Status, obs.Message)
	}
	if obs.Payload["cpu_percent"] != float64(41) || obs.Payload["memory_percent"] != float64(73) {
		t.Fatalf("cpu/mem canonicos inesperados: cpu=%#v mem=%#v", obs.Payload["cpu_percent"], obs.Payload["memory_percent"])
	}
	cpuMemory, ok := obs.Payload["cpu_memory"].(map[string]any)
	if !ok {
		t.Fatalf("cpu_memory ausente: %#v", obs.Payload["cpu_memory"])
	}
	samples, ok := cpuMemory["vendor_samples"].([]any)
	if !ok || len(samples) != 2 {
		t.Fatalf("vendor_samples inesperado: %#v", cpuMemory["vendor_samples"])
	}
}

func TestRunJobSNMPAddsSegmentMetadata(t *testing.T) {
	job := newSNMPJob()
	job.CollectionKind = "metrics_fast"
	job.Config["collection_kind"] = "metrics_fast"
	job.Config["segment_id"] = "metrics_fast"
	job.Config["segment_seq"] = 3
	job.Config["fetch_mode"] = "get_only"
	job.Config["time_budget_ms"] = 60000
	job.Config["snmp_max_rows"] = 1000
	job.Config["include_groups"] = []any{"system", "custom"}
	job.Config["exclude_groups"] = []any{"interfaces_status", "interfaces_counters"}

	withFakeSNMPClient(t, &fakeSNMPClient{
		getFn: func(oids []string) (*gosnmp.SnmpPacket, error) {
			return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{
				{Name: "1.3.6.1.2.1.1.1.0", Type: gosnmp.OctetString, Value: []byte("switch")},
			}}, nil
		},
	})

	obs := RunJob(context.Background(), job)
	if obs.CollectionKind != "metrics_fast" {
		t.Fatalf("collection_kind inesperado: %q", obs.CollectionKind)
	}
	if obs.SegmentID != "metrics_fast" {
		t.Fatalf("segment_id inesperado: %q", obs.SegmentID)
	}
	if obs.SegmentSeq != 3 {
		t.Fatalf("segment_seq inesperado: %d", obs.SegmentSeq)
	}
	if obs.IsPartial {
		t.Fatalf("is_partial deveria ser false")
	}
	if !obs.IsFinal {
		t.Fatalf("is_final deveria ser true")
	}
	if obs.SegmentStartedAt == nil || obs.SegmentStartedAt.IsZero() {
		t.Fatalf("segment_started_at ausente")
	}
	if obs.DedupeKey == "" {
		t.Fatalf("dedupe_key ausente")
	}
	if obs.Payload["collection_kind"] != "metrics_fast" || obs.Payload["segment_id"] != "metrics_fast" {
		t.Fatalf("payload sem metadados de segmento: %#v", obs.Payload)
	}
	runtime, ok := obs.Payload["runtime_applied"].(map[string]any)
	if !ok {
		t.Fatalf("payload sem runtime_applied: %#v", obs.Payload)
	}
	if runtime["snmp_collection_kind"] != "metrics_fast" || runtime["fetch_mode"] != "get_only" {
		t.Fatalf("runtime_applied inesperado: %#v", runtime)
	}
	if runtime["time_budget_ms"] != float64(60000) || runtime["snmp_max_rows"] != float64(1000) {
		t.Fatalf("runtime_applied sem limites efetivos: %#v", runtime)
	}
	if runtime["collection_profile"] != "minimal" {
		t.Fatalf("runtime_applied sem profile efetivo: %#v", runtime)
	}
	if includeGroups, ok := runtime["include_groups"].([]any); !ok || len(includeGroups) != 2 || includeGroups[0] != "system" || includeGroups[1] != "custom" {
		t.Fatalf("runtime_applied sem include_groups: %#v", runtime["include_groups"])
	}
	if excludeGroups, ok := runtime["exclude_groups"].([]any); !ok || len(excludeGroups) != 2 || excludeGroups[0] != "interfaces_status" || excludeGroups[1] != "interfaces_counters" {
		t.Fatalf("runtime_applied sem exclude_groups: %#v", runtime["exclude_groups"])
	}
	if groups, ok := runtime["groups_requested"].([]any); !ok || len(groups) != 2 || groups[0] != "system" || groups[1] != "custom" {
		t.Fatalf("runtime_applied sem grupos finais: %#v", runtime["groups_requested"])
	}
	if obs.Payload["dedupe_key"] != obs.DedupeKey {
		t.Fatalf("payload dedupe_key inesperado: %#v", obs.Payload["dedupe_key"])
	}
}

func TestRunJobWithPartialsEmitsSNMPGroupObservations(t *testing.T) {
	job := newSNMPJob()
	job.CollectionKind = "metrics_fast"
	job.Config["collection_kind"] = "metrics_fast"
	job.Config["groups"] = []any{"system", "ip_stats"}
	job.Config["fetch_mode"] = "get_only"

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

	var partials []entities.AgentlessObservation
	finalObs := RunJobWithPartials(context.Background(), job, func(obs entities.AgentlessObservation) {
		partials = append(partials, obs)
	})
	if len(partials) != 2 {
		t.Fatalf("esperava 2 observacoes parciais, obtido %d", len(partials))
	}
	for i, partial := range partials {
		if !partial.IsPartial || partial.IsFinal {
			t.Fatalf("flags parciais invalidas: %#v", partial)
		}
		if partial.SegmentSeq != i+1 {
			t.Fatalf("segment_seq parcial inesperado: %d", partial.SegmentSeq)
		}
		if partial.DedupeKey == "" {
			t.Fatalf("dedupe_key parcial ausente")
		}
		if partial.Payload["segment_group"] == "" {
			t.Fatalf("segment_group parcial ausente: %#v", partial.Payload)
		}
	}
	if finalObs.IsPartial || !finalObs.IsFinal {
		t.Fatalf("flags finais invalidas: %#v", finalObs)
	}
	if finalObs.SegmentSeq != 3 {
		t.Fatalf("segment_seq final inesperado: %d", finalObs.SegmentSeq)
	}
	if finalObs.DedupeKey == "" || finalObs.DedupeKey == partials[0].DedupeKey {
		t.Fatalf("dedupe_key final invalida: %q", finalObs.DedupeKey)
	}
}
