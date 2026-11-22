package monitoring

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type transportCase struct {
	Name       string              `json:"name"`
	Definition Definition          `json:"definition"`
	Authority  DefinitionAuthority `json:"authority"`
	Now        int64               `json:"now_ms"`
	Batch      RecordsBatch        `json:"batch"`
	Raw        *string             `json:"raw,omitempty"`
	Hops       []TransportProfile  `json:"hops"`
	Valid      bool                `json:"valid"`
}

func transportCases(t *testing.T) []transportCase {
	t.Helper()
	raw, err := os.ReadFile("testdata/transport-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []transportCase
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	return cases
}

func transportRaw(t *testing.T, c transportCase) []byte {
	t.Helper()
	if c.Raw != nil {
		return []byte(*c.Raw)
	}
	raw, err := json.Marshal(c.Batch)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestTransportAllSourcesTypesAndCompatibility(t *testing.T) {
	for _, c := range transportCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			before, _ := json.Marshal(c)
			b, err := DecodeTransportRecords(transportRaw(t, c), c.Definition, c.Authority, c.Now, c.Hops)
			if c.Raw == nil {
				_, encodeErr := EncodeTransportRecords(c.Batch, c.Definition, c.Authority, c.Now, c.Hops)
				if (encodeErr == nil) != c.Valid {
					t.Fatalf("encode acceptance mismatch: %v", encodeErr)
				}
			}
			if (err == nil) != c.Valid {
				t.Fatalf("acceptance mismatch: %v", err)
			}
			if err == nil {
				raw, e := EncodeTransportRecords(b, c.Definition, c.Authority, c.Now, c.Hops)
				if e != nil {
					t.Fatal(e)
				}
				again, e := DecodeTransportRecords(raw, c.Definition, c.Authority, c.Now, c.Hops)
				if e != nil || !reflect.DeepEqual(b, again) {
					t.Fatal("roundtrip changed typed records", e)
				}
			} else if strings.Contains(err.Error(), "fixture-secret") {
				t.Fatal("payload leaked")
			}
			after, _ := json.Marshal(c)
			if !bytes.Equal(before, after) {
				t.Fatal("inputs mutated")
			}
		})
	}
}

func TestTransportActualPHPParity(t *testing.T) {
	root := os.Getenv("AICEBERG_MONITORING_WEB_ROOT")
	if root == "" {
		t.Skip("explicit Web checkout required")
	}
	cases := transportCases(t)
	input, err := json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("php", filepath.Join(root, "common/tests/_support/monitoring-transport.php"))
	cmd.Stdin = bytes.NewReader(input)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("PHP helper: %v %s", err, output)
	}
	var results []struct {
		Valid bool   `json:"valid"`
		Raw   string `json:"raw"`
	}
	if json.Unmarshal(output, &results) != nil || len(results) != len(cases) {
		t.Fatal("invalid PHP receipt")
	}
	for i, c := range cases {
		if results[i].Valid != c.Valid {
			t.Fatalf("PHP mismatch: %s", c.Name)
		}
		if !c.Valid {
			continue
		}
		got, e := DecodeTransportRecords([]byte(results[i].Raw), c.Definition, c.Authority, c.Now, c.Hops)
		if e != nil || !reflect.DeepEqual(c.Batch, got) {
			t.Fatalf("PHP changed %s: %v", c.Name, e)
		}
	}
	local, _ := os.ReadFile("testdata/transport-v1.json")
	peer, err := os.ReadFile(filepath.Join(root, "docs/contracts/fixtures/monitoring-transport-v1.json"))
	if err != nil || !bytes.Equal(local, peer) {
		t.Fatal("shared fixture drift", err)
	}
}

func TestTransportNegotiationDoesNotAliasProfiles(t *testing.T) {
	c := transportCases(t)[0]
	c.Hops[1].MaxRecords = 7
	c.Hops[1].MaxBytes = 4096
	c.Hops[1].Sources = []string{"snmp"}
	c.Hops[1].ValueTypes = []string{"number"}
	selected, err := NegotiateRecordsTransport(c.Hops)
	if err != nil || selected.MaxRecords != 7 || selected.MaxBytes != 4096 || !reflect.DeepEqual(selected.Sources, []string{"snmp"}) {
		t.Fatal(selected, err)
	}
	selected.Sources[0] = "host"
	selected.ValueTypes[0] = "event"
	if c.Hops[0].Sources[0] != "snmp" || c.Hops[1].ValueTypes[0] != "number" {
		t.Fatal("negotiation aliases policy")
	}
}

type countingTransportReader struct {
	read int
	err  error
}

func (r *countingTransportReader) Read(p []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	for i := range p {
		p[i] = ' '
	}
	r.read += len(p)
	return len(p), nil
}

func TestTransportReaderBoundsAndSafeErrors(t *testing.T) {
	c := transportCases(t)[0]
	c.Hops[1].MaxBytes = 19
	reader := &countingTransportReader{}
	if _, err := ReadTransportRecords(reader, c.Definition, c.Authority, c.Now, c.Hops); err == nil || err.Error() != "monitoring_transport_limit" || reader.read != 20 {
		t.Fatal("unbounded read", reader.read, err)
	}
	reader = &countingTransportReader{err: io.ErrUnexpectedEOF}
	if _, err := ReadTransportRecords(reader, c.Definition, c.Authority, c.Now, c.Hops); err == nil || err.Error() != "invalid_monitoring_transport_body" {
		t.Fatal(err)
	}
	if _, err := ReadTransportRecords(nil, c.Definition, c.Authority, c.Now, c.Hops); err == nil {
		t.Fatal("nil reader accepted")
	}
	reader = &countingTransportReader{}
	if _, err := ReadTransportRecords(reader, c.Definition, c.Authority, c.Now, nil); err == nil || reader.read != 0 {
		t.Fatal("read before negotiation")
	}
}

func TestTransportExactByteAndRecordLimits(t *testing.T) {
	c := transportCases(t)[0]
	raw := transportRaw(t, c)
	c.Hops[1].MaxBytes = len(raw)
	if _, err := DecodeTransportRecords(raw, c.Definition, c.Authority, c.Now, c.Hops); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeTransportRecords(append(raw, ' '), c.Definition, c.Authority, c.Now, c.Hops); err == nil {
		t.Fatal("byte limit exceeded")
	}
	c.Hops[1].MaxBytes = MaxRecordBytes
	original := c.Batch.Records[0]
	for i := 1; i < MaxRecords; i++ {
		r := original
		r.ObservedAtMS -= int64(i)
		c.Batch.Records = append(c.Batch.Records, r)
	}
	if _, err := EncodeTransportRecords(c.Batch, c.Definition, c.Authority, c.Now, c.Hops); err != nil {
		t.Fatal(err)
	}
	c.Hops[1].MaxRecords = MaxRecords - 1
	if _, err := EncodeTransportRecords(c.Batch, c.Definition, c.Authority, c.Now, c.Hops); err == nil || err.Error() != "monitoring_transport_limit" {
		t.Fatal("record cap exceeded", err)
	}
	c.Hops[1].MaxRecords = MaxRecords
	c.Batch.Records = append(c.Batch.Records, original)
	if _, err := EncodeTransportRecords(c.Batch, c.Definition, c.Authority, c.Now, c.Hops); err == nil {
		t.Fatal("global cap exceeded")
	}
}
