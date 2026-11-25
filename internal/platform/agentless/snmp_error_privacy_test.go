package agentless

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestSNMPErrorDetailsStayOutOfPartialAndFinalObservations(t *testing.T) {
	const private = "fixture-private-error-marker"
	for _, scenario := range []struct {
		name   string
		err    error
		code   string
		reason string
	}{
		{"timeout", fmt.Errorf("%s: %w", private, context.DeadlineExceeded), "snmp_timeout", "deadline"},
		{"authentication", fmt.Errorf("%s: %w", private, gosnmp.ErrWrongDigest), "snmp_auth", "configuration"},
		{"refused", errors.New("connection refused: " + private), "snmp_refused", "connection"},
		{"transport", errors.New("invalid transport response: " + private), "snmp_fail", "execution"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			job := newSNMPJob()
			job.ClienteID, job.AtivoID = 2, 4
			job.Config["flush_partial"] = true
			job.Config["custom_get_oids"] = []string{"1.3.6.1.2.1.1.3.0"}
			job.Config["custom_walk_oids"] = []string{"1.3.6.1.2.1.2"}
			walk := func(string, gosnmp.WalkFunc) error { return scenario.err }
			withFakeSNMPClient(t, &fakeSNMPClient{
				getFn:      func([]string) (*gosnmp.SnmpPacket, error) { return nil, scenario.err },
				bulkWalkFn: walk,
				walkFn:     walk,
			})
			var partials []entities.AgentlessObservation
			final := RunJobWithPartials(context.Background(), job, func(obs entities.AgentlessObservation) {
				partials = append(partials, obs)
			})
			if len(partials) == 0 || final.Status != "fail" || final.Code != scenario.code || final.ProbeReport == nil {
				t.Fatalf("lost partials or canonical failure: parts=%d code=%s", len(partials), final.Code)
			}
			if final.ProbeReport.Reason != scenario.reason {
				t.Fatalf("unexpected canonical reason: got=%s want=%s", final.ProbeReport.Reason, scenario.reason)
			}
			for _, obs := range append(partials, final) {
				wire, err := json.Marshal(obs)
				if err != nil || strings.Contains(string(wire), private) {
					t.Fatal("raw SNMP error escaped collection")
				}
			}
			failed, _ := final.Payload["oids_failed"].([]any)
			if len(failed) != 2 {
				t.Fatalf("GET/WALK failure details were not retained by OID: count=%d", len(failed))
			}
		})
	}
}
