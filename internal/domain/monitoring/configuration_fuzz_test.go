package monitoring

import (
	"encoding/json"
	"os"
	"testing"
)

func FuzzConfigurationNeverPanics(f *testing.F) {
	raw, err := os.ReadFile("testdata/configuration-v1.json")
	if err != nil {
		f.Fatal(err)
	}
	var cases []configurationCase
	if json.Unmarshal(raw, &cases) != nil || len(cases) == 0 {
		f.Fatal("invalid fixture")
	}
	c := cases[0]
	f.Add([]byte(c.Wire))
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Fuzz(func(t *testing.T, input []byte) {
		config, err := DecodeConfiguration(input, c.Authority)
		if err != nil {
			return
		}
		out, err := EncodeConfiguration(config, c.Authority)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeConfiguration(out, c.Authority); err != nil {
			t.Fatal(err)
		}
	})
}
