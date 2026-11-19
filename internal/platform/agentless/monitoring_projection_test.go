package agentless

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	gosnmp "github.com/gosnmp/gosnmp"
)

func TestCanonicalProjectionDoesNotRecollectOrChangeLegacyObservation(t *testing.T) {
	calls := 0
	job := newSNMPJob()
	job.ClienteID = 3
	job.AtivoID = 52
	job.Config["fetch_mode"] = "get_only"
	withFakeSNMPClient(t, &fakeSNMPClient{getFn: func(oids []string) (*gosnmp.SnmpPacket, error) {
		calls++
		return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: oids[0], Type: gosnmp.Integer, Value: 30}}}, nil
	}})
	observation := RunJob(context.Background(), job)
	// Values already obtained by a canonical-capable collector; no synthetic wire emission.
	observation.Payload["cpu_percent"] = 80.0
	observation.Payload["memory_percent"] = 40.0
	before, _ := json.Marshal(observation)
	collected := calls
	first, err := ProjectSNMPObservation(job, observation, observation.ObservedAt, 60)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ProjectSNMPObservation(job, observation, observation.ObservedAt, 60)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(observation)
	if calls != collected || collected == 0 || string(before) != string(after) {
		t.Fatal("projection recollected or changed legacy observation")
	}
	if len(first.Samples) != 2 || first.Samples[0].State != "ok" || first.Samples[0].ItemKey != second.Samples[0].ItemKey {
		t.Fatalf("unstable fan-out: %+v", first)
	}
	// Execution location is not part of item identity; authority is still the original job/check.
	job.Config["collector_id"] = 999
	delegated, err := ProjectSNMPObservation(job, observation, observation.ObservedAt, 60)
	if err != nil || delegated.Samples[0].ItemKey != first.Samples[0].ItemKey {
		t.Fatal("collector location changed series identity")
	}
	observation.CheckID++
	if _, err := ProjectSNMPObservation(job, observation, time.Now(), 60); err == nil {
		t.Fatal("accepted another check")
	}
}
