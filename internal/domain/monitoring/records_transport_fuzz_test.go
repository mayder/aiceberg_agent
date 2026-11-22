package monitoring

import (
	"encoding/json"
	"os"
	"testing"
)

func FuzzRecordsTransportNeverPanics(f *testing.F) {
	raw, err := os.ReadFile("testdata/transport-v1.json")
	if err != nil {
		f.Fatal(err)
	}
	var cases []transportCase
	if json.Unmarshal(raw, &cases) != nil || len(cases) == 0 {
		f.Fatal("invalid fixtures")
	}
	c := cases[0]
	seed, _ := json.Marshal(c.Batch)
	f.Add(seed)
	f.Add([]byte(`{}`))
	f.Add([]byte("null"))
	f.Fuzz(func(t *testing.T, input []byte) {
		b, err := DecodeTransportRecords(input, c.Definition, c.Authority, c.Now, c.Hops)
		if err != nil {
			return
		}
		encoded, err := EncodeTransportRecords(b, c.Definition, c.Authority, c.Now, c.Hops)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = DecodeTransportRecords(encoded, c.Definition, c.Authority, c.Now, c.Hops); err != nil {
			t.Fatal(err)
		}
	})
}
