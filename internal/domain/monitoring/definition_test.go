package monitoring

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type definitionFixture struct {
	Name          string              `json:"name"`
	Authority     DefinitionAuthority `json:"authority"`
	Definition    json.RawMessage     `json:"definition"`
	ExpectedError string              `json:"expected_error"`
}

func definitionFixtureBytes(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/definitions-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestDefinitionSharedContract(t *testing.T) {
	raw := definitionFixtureBytes(t)
	var cases []definitionFixture
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			before := append([]byte(nil), c.Definition...)
			d, err := DecodeDefinition(c.Definition)
			if err == nil {
				err = ValidateDefinition(d, c.Authority)
			}
			actual := ""
			if err != nil {
				actual = err.Error()
			}
			if actual != c.ExpectedError {
				t.Fatalf("error %q want %q", actual, c.ExpectedError)
			}
			if !bytes.Equal(before, c.Definition) {
				t.Fatal("mutated input")
			}
			if strings.Contains(actual, "fixture-do-not-copy") {
				t.Fatal("input leaked to diagnostic")
			}
			if err == nil {
				encoded, err := json.Marshal(d)
				if err != nil {
					t.Fatal(err)
				}
				again, err := DecodeDefinition(encoded)
				if err != nil || !reflect.DeepEqual(d, again) {
					t.Fatal("roundtrip changed definition")
				}
			}
		})
	}
}

func TestDefinitionPHPInterop(t *testing.T) {
	root := os.Getenv("AICEBERG_MONITORING_WEB_ROOT")
	if root == "" {
		t.Skip("set AICEBERG_MONITORING_WEB_ROOT for PHP interop")
	}
	raw := definitionFixtureBytes(t)
	authority, err := os.ReadFile(filepath.Join(root, "docs/contracts/fixtures/monitoring-definitions-v1.json"))
	if err != nil || !bytes.Equal(raw, authority) {
		t.Fatal("shared definition fixtures drifted")
	}
	var cases []definitionFixture
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			input, _ := json.Marshal(c)
			cmd := exec.Command("php", filepath.Join(root, "common/tests/_support/monitoring-definition.php"))
			cmd.Stdin = bytes.NewReader(input)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("PHP contract: %s", output)
			}
			var result struct {
				Error string `json:"error"`
			}
			if json.Unmarshal(output, &result) != nil || result.Error != c.ExpectedError {
				t.Fatalf("PHP rejected differently: %s", output)
			}
		})
	}
}

func TestDefinitionWireGuards(t *testing.T) {
	var cases []definitionFixture
	if err := json.Unmarshal(definitionFixtureBytes(t), &cases); err != nil {
		t.Fatal(err)
	}
	good := string(cases[0].Definition)
	if _, err := DecodeDefinition([]byte(good)); err != nil {
		t.Fatal(err)
	}
	duplicate := strings.Replace(good, `"schema_version": 1`, `"schema_version":1,"schema_version":1`, 1)
	if duplicate == good {
		t.Fatal("test did not inject duplicate")
	}
	for _, raw := range []string{
		duplicate, good + " {}", good + strings.Repeat(" ", MaxDefinitionBytes),
		`null`, `[]`, `{}`, `{"items":[[[[[[[[[[]]]]]]]]]]}`,
	} {
		if _, err := DecodeDefinition([]byte(raw)); err == nil {
			t.Fatalf("accepted invalid wire shape: %d bytes", len(raw))
		}
	}
}

func TestDefinitionTypedNilDependenciesAreRejected(t *testing.T) {
	var cases []definitionFixture
	if err := json.Unmarshal(definitionFixtureBytes(t), &cases); err != nil {
		t.Fatal(err)
	}
	d, err := DecodeDefinition(cases[0].Definition)
	if err != nil {
		t.Fatal(err)
	}
	d.Items[0].DependsOn = nil
	if err := ValidateDefinition(d, cases[0].Authority); err == nil {
		t.Fatal("nil list accepted")
	}
}

func BenchmarkSNMPCompatibilityBaseline(b *testing.B) {
	scope := Scope{ClientID: 3, AssetID: 52, CheckID: 72}
	observation := Observation{Status: "ok", Final: true, Payload: map[string]any{"cpu_percent": 12.5, "memory_percent": 40.0}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := ProjectSNMP(scope, observation, 1800000000000, 1800000000000, 60); err != nil {
			b.Fatal(err)
		}
	}
}
