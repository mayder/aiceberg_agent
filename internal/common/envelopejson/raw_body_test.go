package envelopejson

import (
	"encoding/json"
	"testing"
)

func TestDecodeBatchKeepsBodyAndRouting(t *testing.T) {
	raw := []byte(`[{"envelope_id":"one","agent_id":"relay","schema_version":1,"endpoint":"/v1/ingest/metrics","meta":{"via":"relay"},"body":{"integer":9007199254740993,"items":[true,null,"x"]}}]`)
	batch, err := DecodeBatch(raw)
	if err != nil || len(batch) != 1 {
		t.Fatal(err)
	}
	e := batch[0]
	if e.ID != "one" || e.AgentID != "relay" || e.Endpoint != "/v1/ingest/metrics" || e.Meta["via"] != "relay" {
		t.Fatal("routing changed")
	}
	b, ok := e.Body.(json.RawMessage)
	if !ok || string(b) != `{"integer":9007199254740993,"items":[true,null,"x"]}` {
		t.Fatal("opaque JSON changed")
	}
	for i := range raw {
		raw[i] = 'x'
	}
	if !json.Valid(b) {
		t.Fatal("body aliases request memory")
	}
}

func TestDecodeBatchRejectsMalformedBodiesAndHandlesNull(t *testing.T) {
	for _, raw := range []string{`[{"body":{"x":1}]`, `{}`, `[{"envelope_id":3}]`} {
		if _, err := DecodeBatch([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, raw := range []string{`null`, `[]`, `[null,{"body":null},{"body":{}}]`} {
		if _, err := DecodeBatch([]byte(raw)); err != nil {
			t.Fatal(err)
		}
	}
}
