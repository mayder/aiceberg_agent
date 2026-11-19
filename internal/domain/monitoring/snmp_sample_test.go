package monitoring

import (
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type fixture struct {
	Name  string `json:"name"`
	Input struct {
		Scope       Scope       `json:"scope"`
		Observation Observation `json:"observation"`
		ObservedMS  int64       `json:"observed_at_ms"`
		NowMS       int64       `json:"now_ms"`
		Freshness   int64       `json:"freshness_sec"`
	} `json:"input"`
	Expected Batch `json:"expected"`
}

func TestSNMPSampleSharedContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/snmp-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []fixture
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			got, err := ProjectSNMP(c.Input.Scope, c.Input.Observation, c.Input.ObservedMS, c.Input.NowMS, c.Input.Freshness)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.Expected) {
				t.Fatalf("got %+v want %+v", got, c.Expected)
			}
			encoded, _ := json.Marshal(got)
			if strings.Contains(string(encoded), "fixture-do-not-copy") {
				t.Fatal("projection copied untrusted fields")
			}
		})
	}
}

func TestSNMPSampleRejectsInvalidAuthorityAndClock(t *testing.T) {
	scope := Scope{ClientID: 3, AssetID: 52, CheckID: 72}
	for _, field := range []string{"client", "asset", "check", "unsafe_integer"} {
		invalid := scope
		switch field {
		case "client":
			invalid.ClientID = 0
		case "asset":
			invalid.AssetID = -1
		case "check":
			invalid.CheckID = 0
		case "unsafe_integer":
			invalid.ClientID = 9007199254740992
		}
		if _, err := ProjectSNMP(invalid, Observation{}, 1800000000000, 1800000000000, 60); err == nil {
			t.Fatalf("accepted %s", field)
		}
	}
	for _, clock := range [][3]int64{{0, 1, 60}, {1, 0, 60}, {1, 1, 0}, {1, 1, 604801}, {253402300800000, 1, 60}, {1, 253402300800000, 60}} {
		if _, err := ProjectSNMP(scope, Observation{}, clock[0], clock[1], clock[2]); err == nil {
			t.Fatalf("accepted clock %v", clock)
		}
	}
	for _, number := range []any{math.NaN(), math.Inf(1), math.Inf(-1), "NaN", "Inf", json.Number("0x1p1"), true, []any{2}, map[string]any{"value": 2}} {
		got, err := ProjectSNMP(scope, Observation{Status: "ok", Final: true, Payload: map[string]any{"cpu_percent": number}}, 1, 1, 60)
		if err != nil || got.Samples[0].State != "unknown" || got.Samples[0].Value != nil {
			t.Fatalf("accepted non-finite or non-numeric %T", number)
		}
	}
}

// Opt-in cross-check uses the actual PHP adapter, without any database or network.
func TestSNMPSamplePHPInterop(t *testing.T) {
	root := os.Getenv("AICEBERG_MONITORING_WEB_ROOT")
	if root == "" {
		t.Skip("set AICEBERG_MONITORING_WEB_ROOT for PHP interop; shared golden fixtures run independently")
	}
	raw, err := os.ReadFile("testdata/snmp-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := os.ReadFile(filepath.Join(root, "docs/contracts/fixtures/monitoring-snmp-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(canonical) {
		t.Fatal("contract fixtures drifted from Web authority")
	}
	var cases []fixture
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			input, _ := json.Marshal(c.Input)
			cmd := exec.Command("php", filepath.Join(root, "common/tests/_support/monitoring-snmp-projection.php"))
			cmd.Stdin = strings.NewReader(string(input))
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("PHP contract failed: %s", output)
			}
			var actual Batch
			if err := json.Unmarshal(output, &actual); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(actual, c.Expected) {
				t.Fatalf("PHP differs: %+v", actual)
			}
		})
	}
}
