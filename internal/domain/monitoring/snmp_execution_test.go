package monitoring

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestDefinedSNMPReusesAllSampleVectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/snmp-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []fixture
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			scope := c.Input.Scope
			authority := DefinitionAuthority{ClientID: scope.ClientID, AssetID: scope.AssetID, CheckIDs: []int64{scope.CheckID}}
			result, err := ProjectDefinedSNMP(SNMPCompatibilityDefinition(scope, c.Input.Freshness), authority,
				c.Input.Observation, c.Input.ObservedMS, c.Input.NowMS)
			if err != nil || !reflect.DeepEqual(result, c.Expected) {
				t.Fatalf("contract drift: %v %+v", err, result)
			}
		})
	}
}

func TestDefinedSNMPRejectsUnsupportedExecutionAndForeignAuthority(t *testing.T) {
	scope := Scope{ClientID: 3, AssetID: 52, CheckID: 72}
	authority := DefinitionAuthority{ClientID: 3, AssetID: 52, CheckIDs: []int64{72}}
	for _, field := range []string{"source", "metric", "type", "unit", "disabled", "freshness", "client", "check", "count", "authority_count"} {
		t.Run(field, func(t *testing.T) {
			d := SNMPCompatibilityDefinition(scope, 60)
			a := authority
			switch field {
			case "count":
				d.Items = d.Items[:1]
			case "authority_count":
				a.CheckIDs = []int64{72, 73}
			case "source":
				d.Items[0].Source = "http"
			case "metric":
				d.Items[0].Metric = "temperature"
			case "type":
				d.Items[0].ValueType = "integer"
			case "unit":
				d.Items[0].Unit = "celsius"
			case "disabled":
				d.Items[0].Enabled = false
			case "freshness":
				d.Items[1].FreshnessSec = 61
			case "client":
				a.ClientID = 4
			case "check":
				a.CheckIDs = []int64{73}
			}
			_, err := ProjectDefinedSNMP(d, a, Observation{Status: "ok", Final: true}, 1800000000000, 1800000000000)
			if err == nil {
				t.Fatal("unsupported or unauthorized definition executed")
			}
		})
	}
}
