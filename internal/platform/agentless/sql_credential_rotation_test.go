package agentless

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

const rotatedSQLFixturePassword = "fixture-rotated-sql-password"

func privateSQLFixtureDir(t *testing.T, profilePath, prefix string) string {
	t.Helper()
	if os.Getenv("AICEBERG_SQL_ROTATION_FIXTURE") != "1" {
		return ""
	}
	dir := filepath.Dir(profilePath)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 || !strings.HasPrefix(filepath.Base(dir), prefix) {
		t.Fatal("credential rotation requires an isolated private fixture")
	}
	return dir
}

func verifySQLProbeRotation(t *testing.T, job entities.AgentlessJob, path string, updated any, rejectedCode, previousPassword string, sampleCount int) {
	t.Helper()
	stale := RunJob(context.Background(), job)
	if stale.Status != "fail" || stale.Code != rejectedCode || len(stale.Payload) != 0 ||
		stale.ProbeReport == nil || stale.ProbeReport.Outcome != "failed" || stale.ProbeReport.Reason != "configuration" {
		t.Fatalf("revoked credential did not fail closed: %s", stale.Code)
	}
	if stale.MonitoringSamples != nil {
		for _, sample := range stale.MonitoringSamples.Samples {
			if sample.Value != nil {
				t.Fatal("revoked credential produced a numeric sample")
			}
		}
	}
	raw, err := json.Marshal(map[string]any{"profiles": []any{updated}})
	if err != nil {
		t.Fatal("encode replacement fixture profile")
	}
	replacement, err := os.CreateTemp(filepath.Dir(path), ".rotated-profile-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(replacement.Name())
	_, writeErr := replacement.Write(raw)
	closeErr := replacement.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal("write replacement fixture profile")
	}
	if err := os.Rename(replacement.Name(), path); err != nil {
		t.Fatal(err)
	}
	recovered := RunJob(context.Background(), job)
	if recovered.Status != "ok" || recovered.ProbeReport == nil || recovered.ProbeReport.Outcome != "ok" ||
		recovered.MonitoringSamples == nil || len(recovered.MonitoringSamples.Samples) != sampleCount {
		t.Fatalf("same probe did not recover after atomic profile replacement: %s", recovered.Code)
	}
	for _, sample := range recovered.MonitoringSamples.Samples {
		if sample.Value == nil {
			t.Fatal("recovered probe failed to produce a numeric sample")
		}
	}
	for _, obs := range []entities.AgentlessObservation{stale, recovered} {
		wire, err := json.Marshal(obs)
		if err != nil || bytes.Contains(wire, []byte(rotatedSQLFixturePassword)) || bytes.Contains(wire, []byte(previousPassword)) {
			t.Fatal("SQL probe observation exposed a credential")
		}
	}
	t.Log("old credential refused; atomic profile replacement restored all samples in the same process")
}
