package agentless

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

type nativePipelineFixture struct {
	Configuration          monitoring.Configuration          `json:"configuration"`
	ConfigurationAuthority monitoring.ConfigurationAuthority `json:"configuration_authority"`
	Wire                   string                            `json:"pipeline_wire"`
	Authority              monitoring.PipelineAuthority      `json:"pipeline_authority"`
	Budget                 monitoring.PipelineBudget         `json:"budget"`
}

func pipelineOneTCPObservation(t *testing.T) entities.AgentlessObservation {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(chan bool, 1)
	go func() {
		c, err := listener.Accept()
		if err == nil {
			c.Close()
		}
		accepted <- err == nil
	}()
	observation := RunJob(context.Background(), canonicalTCPJob(listener.Addr().(*net.TCPAddr).Port))
	select {
	case ok := <-accepted:
		if !ok {
			t.Error("connection not accepted")
		}
	case <-time.After(2 * time.Second):
		t.Error("no real TCP connection")
	}
	listener.Close()
	if observation.Status != "ok" || observation.MonitoringSamples == nil {
		t.Fatal("native TCP observation failed")
	}
	return observation
}

func nativePipeline(t *testing.T) nativePipelineFixture {
	t.Helper()
	raw, err := os.ReadFile("../../domain/monitoring/testdata/pipeline-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []nativePipelineFixture
	if json.Unmarshal(raw, &cases) != nil {
		t.Fatal("invalid fixture")
	}
	c := cases[1]
	key := "v1:c:3:a:52:check:73:tcp:latency_us"
	d := &c.Configuration.Definition.Items[0]
	d.Key = key
	d.CheckID = 73
	d.Source = "tcp"
	d.Metric = "latency_us"
	d.Unit = "microseconds"
	item := &c.Configuration.Items[0]
	item.ItemKey = key
	values := map[string]string{"tipo": "tcp", "source": "tcp", "metric": "latency_us", "unit": "microseconds", "configuration_ref": "check-config:v1:3:1:10:tcp:" + strings.Repeat("a", 64)}
	for name, value := range values {
		encoded, _ := json.Marshal(value)
		item.Parameters[name] = monitoring.ConfigurationValue{Type: "text", Value: encoded}
	}
	c.ConfigurationAuthority.Definition.CheckIDs = []int64{73}
	binding := c.ConfigurationAuthority.Bindings[item.TemplateKey]
	binding.CheckID = 73
	c.ConfigurationAuthority.Bindings[item.TemplateKey] = binding
	spec := monitoring.PipelineDefinition{Contract: "monitoring.pipeline.v1", SchemaVersion: 1, ID: "pipeline1", Revision: 1, ClientID: 3, InputType: "integer", InputUnit: "milliseconds", OutputType: "integer", OutputUnit: "microseconds", Steps: []monitoring.PipelineStep{{ID: "scale1", Operation: "scale", InputType: "integer", InputUnit: "milliseconds", OutputType: "integer", OutputUnit: "microseconds", Argument: json.RawMessage("1000")}}}
	wire, _ := json.Marshal(spec)
	c.Wire = string(wire)
	sum := sha256.Sum256(wire)
	c.Authority.SHA256 = hex.EncodeToString(sum[:])
	c.Authority.ItemKeys = []string{key}
	return c
}

func TestPipelineTransformsRealTCPOnceAndPHPConsumesTypedResult(t *testing.T) {
	root := os.Getenv("AICEBERG_MONITORING_WEB_ROOT")
	if root == "" {
		t.Skip("explicit Web checkout required")
	}
	observation := pipelineOneTCPObservation(t)
	c := nativePipeline(t)
	p, err := monitoring.CompilePipeline([]byte(c.Wire), c.Authority)
	if err != nil {
		t.Fatal(err)
	}
	value, _ := json.Marshal(observation.LatencyMs)
	now := observation.ObservedAt.UnixMilli()
	batch := monitoring.RecordsBatch{Contract: "monitoring.records.v1", SchemaVersion: 1, DefinitionRevision: 1, ClientID: 3, AssetID: 52, EvaluatedAtMS: now, Coverage: "complete", Records: []monitoring.TypedRecord{{ItemKey: c.Authority.ItemKeys[0], ValueType: "integer", Unit: "milliseconds", ObservedAtMS: now, State: "ok", Reason: "none", Value: value}}}
	raw, _ := monitoring.EncodePipelineInput(batch, c.Authority)
	result, err := monitoring.RunConfiguredPipeline(context.Background(), c.Configuration, c.ConfigurationAuthority, p, c.Budget, raw, now)
	if err != nil || string(result.Batch.Records[0].Value) != stringMustMarshal(observation.LatencyMs*1000) {
		t.Fatal("native pipeline result mismatch", err)
	}
	again, err := monitoring.RunConfiguredPipeline(context.Background(), c.Configuration, c.ConfigurationAuthority, p, c.Budget, raw, now+86400000)
	if err != nil || !reflect.DeepEqual(result, again) {
		t.Fatal("retry mutated historical data or recollected", err)
	}
	consumeNativePipelinePHP(t, root, c, result, now)
}

func stringMustMarshal(v any) string { raw, _ := json.Marshal(v); return string(raw) }

func consumeNativePipelinePHP(t *testing.T, root string, c nativePipelineFixture, result monitoring.PipelineExecution, now int64) {
	t.Helper()
	request := []map[string]any{{"pipeline_wire": c.Wire, "pipeline_authority": c.Authority, "configuration": c.Configuration, "configuration_authority": c.ConfigurationAuthority, "result": result, "now_ms": now}}
	raw, _ := json.Marshal(request)
	cmd := exec.Command("php", filepath.Join(root, "common/tests/_support/monitoring-pipeline.php"))
	cmd.Stdin = bytes.NewReader(raw)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal("native PHP consumer failed", err)
	}
	var receipts []struct {
		Valid    bool `json:"valid"`
		Consumed bool `json:"consumed"`
	}
	if json.Unmarshal(output, &receipts) != nil || len(receipts) != 1 || !receipts[0].Valid || !receipts[0].Consumed {
		t.Fatal("native pipeline result rejected")
	}
}
