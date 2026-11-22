package monitoring

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestTCPSharedCompatibilityVectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/tcp-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name  string `json:"name"`
		Input struct {
			Scope       Scope       `json:"scope"`
			Observation Observation `json:"observation"`
			Latency     any         `json:"latency_ms"`
			Observed    int64       `json:"observed_at_ms"`
			Now         int64       `json:"now_ms"`
			Freshness   int64       `json:"freshness_sec"`
		} `json:"input"`
		Expected Batch `json:"expected"`
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			d := TCPCompatibilityDefinition(c.Input.Scope, c.Input.Freshness)
			a := DefinitionAuthority{ClientID: c.Input.Scope.ClientID, AssetID: c.Input.Scope.AssetID, CheckIDs: []int64{c.Input.Scope.CheckID}}
			got, err := ProjectDefinedTCP(d, a, c.Input.Observation, c.Input.Latency, c.Input.Observed, c.Input.Now)
			if err != nil || !reflect.DeepEqual(got, c.Expected) {
				t.Fatalf("projection mismatch: %+v %v", got, err)
			}
		})
	}
	if root := os.Getenv("AICEBERG_MONITORING_WEB_ROOT"); root != "" {
		peer, err := os.ReadFile(filepath.Join(root, "docs/contracts/fixtures/monitoring-tcp-v1.json"))
		if err != nil || !bytes.Equal(peer, raw) {
			t.Fatal("shared TCP vectors drifted", err)
		}
	}
}

func TestTCPRejectsUnsafeDefinitionAndClocks(t *testing.T) {
	scope := Scope{ClientID: 3, AssetID: 52, CheckID: 73}
	a := DefinitionAuthority{ClientID: 3, AssetID: 52, CheckIDs: []int64{73}}
	for _, change := range []func(*ItemDefinition){func(i *ItemDefinition) { i.Source = "http" }, func(i *ItemDefinition) { i.Metric = "other" }, func(i *ItemDefinition) { i.Unit = "percent" }, func(i *ItemDefinition) { i.Enabled = false }, func(i *ItemDefinition) { i.ValueType = "number" }, func(i *ItemDefinition) { i.CheckID = 74 }} {
		d := TCPCompatibilityDefinition(scope, 60)
		change(&d.Items[0])
		if _, err := ProjectDefinedTCP(d, a, Observation{}, 0, 1800000000000, 1800000000000); err == nil {
			t.Fatal("unsupported definition accepted")
		}
	}
	for _, clock := range []int64{0, -1, 253402300800000} {
		if _, err := ProjectDefinedTCP(TCPCompatibilityDefinition(scope, 60), a, Observation{}, 0, clock, 1800000000000); err == nil {
			t.Fatal("invalid clock accepted")
		}
	}
	for _, bad := range []any{math.NaN(), math.Inf(1), float64(1.5), true, []any{}, json.Number("1e999")} {
		if _, ok := tcpMilliseconds(bad); ok {
			t.Fatal("invalid latency accepted")
		}
	}
	a.ClientID = 4
	if _, err := ProjectDefinedTCP(TCPCompatibilityDefinition(scope, 60), a, Observation{}, 0, 1800000000000, 1800000000000); err == nil {
		t.Fatal("foreign authority accepted")
	}
}
