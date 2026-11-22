package monitoring

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

type configurationCase struct {
	Name          string                 `json:"name"`
	Configuration Configuration          `json:"-"`
	Wire          json.RawMessage        `json:"configuration"`
	Authority     ConfigurationAuthority `json:"authority"`
	Valid         bool                   `json:"valid"`
}

func configurationCases(t *testing.T) []configurationCase {
	t.Helper()
	raw, err := os.ReadFile("testdata/configuration-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []configurationCase
	if json.Unmarshal(raw, &cases) != nil {
		t.Fatal("invalid fixture")
	}
	for i := range cases {
		if json.Unmarshal(cases[i].Wire, &cases[i].Configuration) != nil {
			t.Fatal("invalid config fixture")
		}
	}
	return cases
}

func TestConfigurationSharedSourcesTypesBindingsAndRoundTrip(t *testing.T) {
	for _, c := range configurationCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			before := append([]byte{}, c.Wire...)
			parsed, err := DecodeConfiguration(before, c.Authority)
			if (err == nil) != c.Valid {
				t.Fatalf("acceptance mismatch: %v", err)
			}
			if err == nil {
				raw, e := EncodeConfiguration(parsed, c.Authority)
				if e != nil {
					t.Fatal(e)
				}
				again, e := DecodeConfiguration(raw, c.Authority)
				if e != nil || !reflect.DeepEqual(parsed, again) {
					t.Fatal("roundtrip changed config", e)
				}
			}
			after := c.Wire
			if !bytes.Equal(before, after) {
				t.Fatal("input mutated")
			}
		})
	}
}

func TestConfigurationActualPHPParity(t *testing.T) {
	root := os.Getenv("AICEBERG_MONITORING_WEB_ROOT")
	if root == "" {
		t.Skip("explicit Web checkout required")
	}
	cases := configurationCases(t)
	raw, _ := json.Marshal(cases)
	cmd := exec.Command("php", filepath.Join(root, "common/tests/_support/monitoring-configuration.php"))
	cmd.Stdin = bytes.NewReader(raw)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("PHP helper failed: %v %s", err, output)
	}
	var results []struct {
		Valid bool   `json:"valid"`
		Raw   string `json:"raw"`
	}
	if json.Unmarshal(output, &results) != nil || len(results) != len(cases) {
		t.Fatal("invalid PHP result")
	}
	for i, c := range cases {
		if results[i].Valid != c.Valid {
			t.Fatalf("PHP acceptance: %s", c.Name)
		}
		if c.Valid {
			if _, err := DecodeConfiguration([]byte(results[i].Raw), c.Authority); err != nil {
				t.Fatal(err)
			}
		}
	}
	local, _ := os.ReadFile("testdata/configuration-v1.json")
	peer, err := os.ReadFile(filepath.Join(root, "docs/contracts/fixtures/monitoring-configuration-v1.json"))
	if err != nil || !bytes.Equal(local, peer) {
		t.Fatal("fixture drift", err)
	}
}

func TestConfigurationStrictWireAndSafeEvolution(t *testing.T) {
	c := configurationCases(t)[0]
	raw, _ := json.Marshal(c.Configuration)
	for _, bad := range [][]byte{append(append([]byte{}, raw...), []byte("{}")...), []byte("null"), []byte("[]"), bytes.Repeat([]byte(" "), MaxDefinitionBytes+1), bytes.Replace(raw, []byte(`"schema_version":1`), []byte(`"schema_version":1,"schema_version":1`), 1)} {
		if _, err := DecodeConfiguration(bad, c.Authority); err == nil {
			t.Fatal("invalid wire accepted")
		}
	}
	next, err := DecodeConfiguration(raw, c.Authority)
	if err != nil {
		t.Fatal(err)
	}
	next.Definition.Revision = 2
	after := c.Authority
	after.Revision = 2
	if err := ValidateConfigurationEvolution(c.Configuration, next, c.Authority, after); err != nil {
		t.Fatal(err)
	}
	next.Definition.Items[0].Unit = "bytes"
	v := next.Items[0].Parameters["unit"]
	v.Value = []byte(`"bytes"`)
	next.Items[0].Parameters["unit"] = v
	if err := ValidateConfigurationEvolution(c.Configuration, next, c.Authority, after); err == nil || err.Error() != "monitoring_series_incompatible" {
		t.Fatal("series type changed", err)
	}
}
