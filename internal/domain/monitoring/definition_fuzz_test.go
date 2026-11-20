package monitoring

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func FuzzDefinitionWire(f *testing.F) {
	raw, err := os.ReadFile("testdata/definitions-v1.json")
	if err != nil {
		f.Fatal(err)
	}
	var cases []definitionFixture
	if err := json.Unmarshal(raw, &cases); err != nil {
		f.Fatal(err)
	}
	f.Add([]byte(cases[0].Definition))
	f.Add([]byte(`{"schema_version":1,"revision":1,"client_id":3,"asset_id":52,"items":[]}`))
	f.Add([]byte(`{"schema_version":1,"schema_version":2}`))
	f.Add([]byte(`null`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		before := append([]byte(nil), raw...)
		definition, err := DecodeDefinition(raw)
		if !bytes.Equal(before, raw) {
			t.Fatal("decoder changed its caller's bytes")
		}
		if err != nil {
			return
		}
		scope := DefinitionAuthority{ClientID: 3, AssetID: 52, CheckIDs: []int64{72, 73, 74}}
		if err := ValidateDefinition(definition, scope); err != nil {
			return
		}
		encoded, err := json.Marshal(definition)
		if err != nil {
			t.Fatal(err)
		}
		again, err := DecodeDefinition(encoded)
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateDefinition(again, scope); err != nil {
			t.Fatal(err)
		}
	})
}
