package monitoring

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type recordCase struct {
	Name       string              `json:"name"`
	Definition Definition          `json:"definition"`
	Authority  DefinitionAuthority `json:"authority"`
	Now        int64               `json:"now_ms"`
	Batch      json.RawMessage     `json:"batch"`
	Valid      bool                `json:"valid"`
}

func recordCases(t *testing.T) []recordCase {
	t.Helper()
	raw, err := os.ReadFile("testdata/records-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []recordCase
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	return cases
}

func TestRecordsSharedTypesStatesAndRoundTrips(t *testing.T) {
	for _, c := range recordCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			batch, err := DecodeRecords(c.Batch)
			if err == nil {
				err = ValidateRecords(batch, c.Definition, c.Authority, c.Now)
			}
			if (err == nil) != c.Valid {
				t.Fatalf("acceptance mismatch: %v", err)
			}
			if err != nil {
				if strings.Contains(err.Error(), "fixture-secret") {
					t.Fatal("payload leaked")
				}
				return
			}
			before, _ := json.Marshal(batch)
			encoded, err := EncodeRecords(batch, c.Definition, c.Authority, c.Now)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := DecodeRecords(encoded)
			if err != nil || ValidateRecords(decoded, c.Definition, c.Authority, c.Now) != nil {
				t.Fatal("roundtrip failed", err)
			}
			after, _ := json.Marshal(batch)
			if !bytes.Equal(before, after) {
				t.Fatal("validator changed values/states")
			}
		})
	}
}

func TestRecordsWireAndPHPParity(t *testing.T) {
	cases := recordCases(t)
	requests := []map[string]any{}
	for _, c := range cases {
		requests = append(requests, map[string]any{"definition": c.Definition, "authority": c.Authority, "now_ms": c.Now, "batch": c.Batch})
	}
	raw, err := os.ReadFile("testdata/records-wire-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var wires []struct {
		Name  string `json:"name"`
		Raw   string `json:"raw"`
		Type  string `json:"value_type"`
		Valid bool   `json:"valid"`
	}
	if json.Unmarshal(raw, &wires) != nil {
		t.Fatal("wire fixture invalid")
	}
	expected := []bool{}
	for _, c := range cases {
		expected = append(expected, c.Valid)
	}
	for _, w := range wires {
		c := recordExample(t, w.Type)
		batch, err := DecodeRecords([]byte(w.Raw))
		if err == nil {
			err = ValidateRecords(batch, c.Definition, c.Authority, c.Now)
		}
		if (err == nil) != w.Valid {
			t.Fatalf("wire %s: %v", w.Name, err)
		}
		requests = append(requests, map[string]any{"definition": c.Definition, "authority": c.Authority, "now_ms": c.Now, "raw": w.Raw})
		expected = append(expected, w.Valid)
	}
	compareRecordsPHP(t, requests, expected)
}

func recordExample(t *testing.T, kind string) recordCase {
	t.Helper()
	for _, c := range recordCases(t) {
		if c.Name == kind+"-ok" {
			return c
		}
	}
	t.Fatal("example missing")
	return recordCase{}
}

func compareRecordsPHP(t *testing.T, requests []map[string]any, expected []bool) {
	t.Helper()
	root := os.Getenv("AICEBERG_MONITORING_WEB_ROOT")
	if root == "" {
		t.Skip("explicit Web checkout required for PHP parity")
	}
	for _, name := range []string{"records-v1", "records-wire-v1", "evolution-v1"} {
		local, err := os.ReadFile("testdata/" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		peer, err := os.ReadFile(filepath.Join(root, "docs/contracts/fixtures/monitoring-"+name+".json"))
		if err != nil || !bytes.Equal(local, peer) {
			t.Fatal("contract fixtures drifted", name, err)
		}
	}
	input, _ := json.Marshal(requests)
	cmd := exec.Command("php", filepath.Join(root, "common/tests/_support/monitoring-records.php"))
	cmd.Stdin = bytes.NewReader(input)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("PHP interop failed: %v %s", err, output)
	}
	var got []struct {
		Valid bool   `json:"valid"`
		Error string `json:"error"`
	}
	if json.Unmarshal(output, &got) != nil || len(got) != len(expected) {
		t.Fatalf("PHP results invalid: %s", output)
	}
	for i, want := range expected {
		if got[i].Valid != want {
			t.Fatalf("PHP parity case%d: %s", i, got[i].Error)
		}
	}
}

func TestRecordsEvolutionAndBounds(t *testing.T) {
	raw, err := os.ReadFile("testdata/evolution-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name      string
		Previous  Definition
		Next      Definition
		Authority DefinitionAuthority
		Valid     bool
	}
	if json.Unmarshal(raw, &cases) != nil {
		t.Fatal("evolution fixture invalid")
	}
	for _, c := range cases {
		if err := ValidateDefinitionEvolution(c.Previous, c.Next, c.Authority); (err == nil) != c.Valid {
			t.Fatalf("evolution %s: %v", c.Name, err)
		}
	}
}

func TestRecordsBounds(t *testing.T) {
	c := recordExample(t, "text")
	batch, _ := DecodeRecords(c.Batch)
	batch.Records[0].Value = json.RawMessage(`"x"`)
	for i := 1; i < 128; i++ {
		r := batch.Records[0]
		r.ObservedAtMS -= int64(i)
		batch.Records = append(batch.Records, r)
	}
	raw, err := EncodeRecords(batch, c.Definition, c.Authority, c.Now)
	if err != nil {
		t.Fatal("bounded128record batch rejected", err)
	}
	if _, err = DecodeRecords(append(raw, bytes.Repeat([]byte(" "), MaxRecordBytes-len(raw))...)); err != nil {
		t.Fatal("wire byte boundary rejected", err)
	}
	for i := range batch.Records {
		batch.Records[i].Value = json.RawMessage(`"` + strings.Repeat("x", 4096) + `"`)
	}
	if _, err := EncodeRecords(batch, c.Definition, c.Authority, c.Now); err == nil {
		t.Fatal("oversize encoding accepted")
	}
	batch.Records = append(batch.Records, batch.Records[0])
	if ValidateRecords(batch, c.Definition, c.Authority, c.Now) == nil {
		t.Fatal("unbounded batch accepted")
	}
	for _, input := range [][]byte{[]byte("{"), []byte("null"), []byte("[]"), {0xff}, []byte(strings.Repeat(" ", MaxRecordBytes+1))} {
		if _, err := DecodeRecords(input); err == nil {
			t.Fatal("invalid wire accepted")
		}
	}

}

func TestRecordsHistoricalQualityAndFreshnessAreIndependent(t *testing.T) {
	c := recordExample(t, "number")
	batch, _ := DecodeRecords(c.Batch)
	batch.EvaluatedAtMS -= 86400000
	batch.Records[0].ObservedAtMS -= 86400000
	if ValidateRecords(batch, c.Definition, c.Authority, c.Now) != nil {
		t.Fatal("historical batch discarded")
	}
	if batch.Records[0].State != "ok" || string(batch.Records[0].Value) != "0" {
		t.Fatal("historical value rewritten")
	}
	if state, err := RecordFreshness(batch.Records[0].ObservedAtMS, 60, c.Now); err != nil || state != "stale" {
		t.Fatal("old quality became current health")
	}
	for _, test := range []struct {
		observed int64
		expected string
	}{{c.Now - 60000, "fresh"}, {c.Now - 60001, "stale"}, {c.Now + 1, "future"}} {
		if got, err := RecordFreshness(test.observed, 60, c.Now); err != nil || got != test.expected {
			t.Fatal("freshness boundary", got, err)
		}
	}
	for _, args := range [][3]int64{{0, 60, c.Now}, {c.Now, 0, c.Now}, {c.Now, 60, 0}} {
		if _, err := RecordFreshness(args[0], args[1], args[2]); err == nil {
			t.Fatal("invalid clock accepted")
		}
	}
}
