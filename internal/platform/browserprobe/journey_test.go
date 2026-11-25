package browserprobe

import (
	"encoding/json"
	"testing"
)

func TestDecodeRejectsUnreviewedActionsAddressesAndArguments(t *testing.T) {
	for name, mutate := range map[string]func(*Journey){
		"rootless popup": func(j *Journey) {
			j.Steps[0].Path = "//other.fixture.example/"
		},
		"external script": func(j *Journey) {
			j.Steps[1].Action = "evaluate"
		},
		"selector expression": func(j *Journey) {
			j.Steps[1].TestID = "x, html"
		},
		"oversize time": func(j *Journey) {
			j.BudgetMS = 30001
		},
		"mismatched IP": func(j *Journey) {
			j.Address = "127.0.0.2"
		},
		"metadata route": func(j *Journey) {
			j.Origin, j.Address = "https://metadata.fixture", "169.254.169.254"
		},
		"insecure remote scheme": func(j *Journey) {
			j.Origin, j.Address = "http://fixture.test", "192.0.2.33"
		},
		"inline origin auth": func(j *Journey) {
			j.Origin = "http://fixture@127.0.0.1"
		},
		"no assertion": func(j *Journey) {
			j.Steps[2] = Step{Action: "click", TestID: "refresh"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			recipe := testRecipe("http://127.0.0.1")
			mutate(&recipe)
			data, err := json.Marshal(recipe)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Decode(data); err == nil {
				t.Fatal("unapproved recipe decoded")
			}
		})
	}
}

func testRecipe(origin string) Journey {
	return Journey{Contract: Contract, ClientID: 3, AssetID: 42, CheckID: 90, EndpointID: 12,
		Origin: origin, Address: "127.0.0.1", BudgetMS: 12000,
		Steps: []Step{
			{Action: "navigate", Path: "/"},
			{Action: "click", TestID: "refresh"},
			{Action: "expect_text", TestID: "state", Text: "Reading updated"},
		},
	}
}
