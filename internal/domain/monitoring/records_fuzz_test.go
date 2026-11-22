package monitoring

import (
	"encoding/json"
	"os"
	"testing"
)

func FuzzRecordsDecodeNeverPanics(f *testing.F) {
	raw, err := os.ReadFile("testdata/records-v1.json")
	if err != nil {
		f.Fatal(err)
	}
	var cases []recordCase
	if json.Unmarshal(raw, &cases) != nil {
		f.Fatal("fixture invalid")
	}
	for _, c := range cases {
		f.Add([]byte(c.Batch))
	}
	f.Add([]byte(`{"records":[null]}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		batch, err := DecodeRecords(raw)
		if err != nil {
			return
		}
		c := cases[0]
		_ = ValidateRecords(batch, c.Definition, c.Authority, c.Now)
	})
}
