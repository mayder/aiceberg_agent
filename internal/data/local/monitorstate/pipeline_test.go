package monitorstate

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	bolt "go.etcd.io/bbolt"
)

type stateCase struct {
	Name            string                            `json:"name"`
	Wire            string                            `json:"pipeline_wire"`
	Authority       monitoring.PipelineAuthority      `json:"pipeline_authority"`
	Config          monitoring.Configuration          `json:"configuration"`
	ConfigAuthority monitoring.ConfigurationAuthority `json:"configuration_authority"`
	Batch           monitoring.RecordsBatch           `json:"batch"`
	Budget          monitoring.PipelineBudget         `json:"budget"`
	Now             int64                             `json:"now_ms"`
}

func fixture(t *testing.T, name string) stateCase {
	t.Helper()
	raw, err := os.ReadFile("../../../domain/monitoring/testdata/pipeline-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []stateCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if c.Name == name {
			c.Budget.TimeoutMS = 2000
			return c
		}
	}
	t.Fatal("missing fixture", name)
	return stateCase{}
}

func (c stateCase) input(t *testing.T, offset int64, value, quality, reason string) (monitoring.RecordsBatch, []byte) {
	t.Helper()
	batch := c.Batch
	batch.Records = append([]monitoring.TypedRecord(nil), batch.Records...)
	batch.EvaluatedAtMS = c.Now + offset
	r := &batch.Records[0]
	r.ObservedAtMS = batch.EvaluatedAtMS
	r.Value, r.State, r.Reason = json.RawMessage(value), quality, reason
	raw, err := monitoring.EncodePipelineInput(batch, c.Authority)
	if err != nil {
		t.Fatal(err)
	}
	return batch, raw
}

func (c stateCase) run(t *testing.T, store *BoltStore, raw []byte) (monitoring.StatefulPipelineExecution, error) {
	t.Helper()
	p, err := monitoring.CompilePipeline([]byte(c.Wire), c.Authority)
	if err != nil {
		t.Fatal(err)
	}
	return monitoring.RunConfiguredStatefulPipeline(context.Background(), c.Config, c.ConfigAuthority, p, c.Budget, raw, c.Now+200000, store)
}

func requireResult(t *testing.T, got monitoring.StatefulPipelineExecution, err error, value, state string) {
	t.Helper()
	if err != nil || got.Result == nil || len(got.Result.Batch.Records) != 1 {
		t.Fatal("missing result", err)
	}
	r := got.Result.Batch.Records[0]
	if string(r.Value) != value || r.State != state {
		t.Fatalf("unexpected result %s/%s", r.Value, r.State)
	}
}

func createState(t *testing.T) (*BoltStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "checkpoint.bolt")
	s, err := Create(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func TestDeltaRestartsAndReplaysWithoutAdvancingTwice(t *testing.T) {
	c := fixture(t, "stateful-counter")
	store, path := createState(t)
	_, first := c.input(t, 1, "100", "ok", "none")
	_, second := c.input(t, 2, "111", "ok", "none")
	_, third := c.input(t, 3, "115", "ok", "none")
	got, err := c.run(t, store, first)
	requireResult(t, got, err, "null", "insufficient_data")
	committed, err := c.run(t, store, second)
	requireResult(t, committed, err, "11", "ok")
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	replay, err := c.run(t, reopened, second)
	if err != nil || !reflect.DeepEqual(committed, replay) {
		t.Fatal("retry changed committed result", err)
	}
	got, err = c.run(t, reopened, third)
	requireResult(t, got, err, "4", "ok")
	_, changed := c.input(t, 2, "999", "ok", "none")
	if got, err = c.run(t, reopened, changed); err == nil || err.Error() != "pipeline_observation_conflict" || got.Result != nil {
		t.Fatal("conflicting replay accepted", err)
	}
}

func TestDeltaCounterResetAndUnavailableRestart(t *testing.T) {
	c := fixture(t, "stateful-counter")
	store, _ := createState(t)
	for i, step := range []struct{ input, quality, reason, value, state string }{
		{"100", "ok", "none", "null", "insufficient_data"},
		{"2", "ok", "none", "null", "insufficient_data"},
		{"5", "ok", "none", "3", "ok"},
		{"null", "unknown", "observation_failed", "null", "unknown"},
		{"12", "ok", "none", "null", "insufficient_data"},
		{"-1", "ok", "none", "null", "unknown"},
		{"20", "ok", "none", "null", "insufficient_data"},
		{"20", "ok", "none", "0", "ok"},
	} {
		_, raw := c.input(t, int64(i+1), step.input, step.quality, step.reason)
		got, err := c.run(t, store, raw)
		requireResult(t, got, err, step.value, step.state)
	}
}

func TestThrottleReplaysSuppressionAndAllowsHeartbeatAndQualityChange(t *testing.T) {
	c := fixture(t, "stateful-throttle-text")
	store, path := createState(t)
	_, first := c.input(t, 1, `"same"`, "ok", "none")
	_, repeated := c.input(t, 2, `"\u0073ame"`, "ok", "none")
	got, err := c.run(t, store, first)
	requireResult(t, got, err, `"same"`, "ok")
	got, err = c.run(t, store, repeated)
	if err != nil || got.Result != nil || got.Suppressed != 1 {
		t.Fatal("repetition emitted a record", got, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	replay, err := c.run(t, reopened, repeated)
	if err != nil || !reflect.DeepEqual(got, replay) {
		t.Fatal("suppression lost across restart", replay, err)
	}
	_, heartbeat := c.input(t, 30001, `"same"`, "ok", "none")
	got, err = c.run(t, reopened, heartbeat)
	requireResult(t, got, err, `"same"`, "ok")
	_, problem := c.input(t, 30002, `"same"`, "problem", "none")
	got, err = c.run(t, reopened, problem)
	requireResult(t, got, err, `"same"`, "problem")
}

func TestPipelineCommitRollsBackOnBudgetFailureAndAcceptsExactRetry(t *testing.T) {
	c := fixture(t, "stateful-counter")
	store, _ := createState(t)
	_, first := c.input(t, 1, "25", "ok", "none")
	_, second := c.input(t, 2, "30", "ok", "none")
	got, err := c.run(t, store, first)
	requireResult(t, got, err, "null", "insufficient_data")
	tight := c
	tight.Budget.MaxOutputBytes = 32
	got, err = tight.run(t, store, second)
	if err == nil || err.Error() != "pipeline_output_limit" || got.Result != nil {
		t.Fatal("output budget did not abort", err)
	}
	got, err = c.run(t, store, second)
	requireResult(t, got, err, "5", "ok")
	if _, err := tight.run(t, store, second); err == nil || err.Error() != "pipeline_output_limit" {
		t.Fatal("replay ignored policy budget", err)
	}
}

func TestPipelineEvictionFencesOldInputAndSerializesDuplicateRuns(t *testing.T) {
	c := fixture(t, "stateful-counter")
	store, _ := createState(t)
	_, original := c.input(t, 1, "1", "ok", "none")
	got, err := c.run(t, store, original)
	requireResult(t, got, err, "null", "insufficient_data")
	for i := int64(2); i <= 9; i++ {
		_, raw := c.input(t, i, "2", "ok", "none")
		if _, err := c.run(t, store, raw); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.run(t, store, original); err == nil || err.Error() != "pipeline_observation_conflict" {
		t.Fatal("evicted input regenerated a result", err)
	}
	_, duplicate := c.input(t, 10, "4", "ok", "none")
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			got, err := c.run(t, store, duplicate)
			if err != nil || got.Result == nil || string(got.Result.Batch.Records[0].Value) != "2" {
				t.Error("duplicate was reprocessed", err)
			}
		}()
	}
	group.Wait()
}

func TestDeltaThrottlePartialBatchIsConsumedByPHP(t *testing.T) {
	root := os.Getenv("AICEBERG_MONITORING_WEB_ROOT")
	if root == "" {
		t.Skip("explicit Web checkout required")
	}
	c := fixture(t, "stateful-delta-throttle")
	store, _ := createState(t)
	batch, _ := c.input(t, 1, "10", "ok", "none")
	seed := batch.Records[0]
	for offset, value := range []string{"12", "14", "15"} {
		record := seed
		record.ObservedAtMS += int64(offset + 1)
		record.Value = json.RawMessage(value)
		batch.Records = append(batch.Records, record)
	}
	batch.EvaluatedAtMS += 3
	raw, err := monitoring.EncodePipelineInput(batch, c.Authority)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.run(t, store, raw)
	if err != nil || got.Suppressed != 1 || got.Result == nil || got.Result.Batch.Coverage != "partial" ||
		len(got.Result.Batch.Records) != 3 || string(got.Result.Batch.Records[1].Value) != "2" ||
		string(got.Result.Batch.Records[2].Value) != "1" {
		t.Fatal("incorrect partial result", got, err)
	}
	request, _ := json.Marshal([]map[string]any{{
		"pipeline_wire": c.Wire, "pipeline_authority": c.Authority, "configuration": c.Config,
		"configuration_authority": c.ConfigAuthority, "batch": batch, "result": got.Result, "now_ms": c.Now + 200000,
	}})
	cmd := exec.Command("php", filepath.Join(root, "common/tests/_support/monitoring-pipeline.php"))
	cmd.Stdin = bytes.NewReader(request)
	if output, err := cmd.CombinedOutput(); err != nil || strings.TrimSpace(string(output)) != `[{"valid":true,"consumed":true}]` {
		t.Fatalf("PHP rejected partial result: %v %s", err, output)
	}
}

func TestBoltCheckpointRejectsMissingScopeChangeAndCorruption(t *testing.T) {
	store, path := createState(t)
	scope := strings.Repeat("a", 64)
	other := strings.Repeat("b", 64)
	if missing, err := Open(path + "-missing"); err == nil {
		_ = missing.Close()
		t.Fatal("missing store was recreated")
	}
	if dup, err := Create(path); err == nil {
		_ = dup.Close()
		t.Fatal("existing store was recreated")
	}
	write := func(_ []byte) ([]byte, error) { return []byte("example"), nil }
	if err := store.UpdatePipelineState(context.Background(), scope, write); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdatePipelineState(context.Background(), other, write); err == nil || err.Error() != "pipeline_checkpoint_scope_mismatch" {
		t.Fatal("other scope accepted", err)
	}
	if err := store.db.Update(func(tx *bolt.Tx) error {
		dest := tx.Bucket(bucket)
		v := bytes.Clone(dest.Get(slot))
		v[len(v)-1] ^= 1
		return dest.Put(slot, v)
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if broken, err := Open(path); err == nil {
		_ = broken.Close()
		t.Fatal("corrupted checkpoint was accepted")
	}
}

func TestCheckpointRecoversAfterProcessExitWithoutClose(t *testing.T) {
	const childPath = "AICEBERG_PIPELINE_TEST_EXIT_PATH"
	c := fixture(t, "stateful-counter")
	_, raw := c.input(t, 2, "13", "ok", "none")
	if path := os.Getenv(childPath); path != "" {
		store, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		got, err := c.run(t, store, raw)
		requireResult(t, got, err, "3", "ok")
		return // Exiting the test process releases the file without Close.
	}
	store, path := createState(t)
	_, first := c.input(t, 1, "10", "ok", "none")
	got, err := c.run(t, store, first)
	requireResult(t, got, err, "null", "insufficient_data")
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCheckpointRecoversAfterProcessExitWithoutClose$")
	cmd.Env = append(os.Environ(), childPath+"="+path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child process failed: %v %s", err, output)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err = c.run(t, reopened, raw)
	requireResult(t, got, err, "3", "ok")
}

func TestDeltaGaugeIsSignedAndIntegerOverflowIsExplicit(t *testing.T) {
	gauge := fixture(t, "stateful-gauge")
	gaugeStore, _ := createState(t)
	_, first := gauge.input(t, 1, "0.5", "ok", "none")
	_, second := gauge.input(t, 2, "-1.25", "ok", "none")
	got, err := gauge.run(t, gaugeStore, first)
	requireResult(t, got, err, "null", "insufficient_data")
	got, err = gauge.run(t, gaugeStore, second)
	requireResult(t, got, err, "-1.75", "ok")

	c := fixture(t, "stateful-delta-throttle")
	store, _ := createState(t)
	_, first = c.input(t, 1, "-9007199254740991", "ok", "none")
	_, second = c.input(t, 2, "9007199254740991", "ok", "none")
	got, err = c.run(t, store, first)
	requireResult(t, got, err, "null", "insufficient_data")
	got, err = c.run(t, store, second)
	requireResult(t, got, err, "null", "unknown")
	if len(got.Result.Failures) != 1 || got.Result.Failures[0].Code != "pipeline_numeric_range" {
		t.Fatal("overflow did not leave an explicit failure")
	}
}

func TestCancelledCheckpointUpdateDoesNotPersist(t *testing.T) {
	store, _ := createState(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scope := strings.Repeat("a", 64)
	err := store.UpdatePipelineState(ctx, scope, func(_ []byte) ([]byte, error) {
		cancel()
		return []byte("must not persist"), nil
	})
	if err != context.Canceled {
		t.Fatal("cancel did not abort checkpoint update", err)
	}
	if err := store.UpdatePipelineState(context.Background(), scope, func(previous []byte) ([]byte, error) {
		if previous != nil {
			t.Error("cancelled update persisted")
		}
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
}
