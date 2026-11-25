package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/relayprobe"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestRelayAssignmentTransportUsesVerifiedHubAndExactResultAcknowledgment(t *testing.T) {
	var exact atomic.Bool
	var calls atomic.Int32
	hub := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		if req.Header.Get("X-Agent-Identity") != "fixture-identity" || req.Header.Get("Authorization") != "Token fixture-token" {
			t.Error("missing relay authentication")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if req.URL.Path == relayprobe.AssignmentResultPath {
			var body struct {
				Observation entities.AgentlessObservation `json:"observation"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Observation.Assignment == nil ||
				body.Observation.Assignment.Epoch != 4 {
				t.Error("result lost the delegated epoch")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			key := "other-result"
			if exact.Load() {
				key = body.Observation.DedupeKey
			}
			fmt.Fprintf(w, `{"status":"ok","accepted":true,"dedupe_key":%q}`, key)
			return
		}
		if req.URL.Path != relayprobe.AssignmentJobPath {
			t.Errorf("unexpected endpoint %q", req.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, `{"status":"offered","check_id":72,"epoch":4}`)
	}))
	defer hub.Close()
	cfg := config.Config{HubURL: hub.URL, APIBaseURL: "http://127.0.0.1:1",
		AgentMode: "relay", Agent: config.AgentCfg{Token: "fixture-token"}}
	client := NewRelaySNMPProbeClientWithIdentity(cfg, func() string { return "fixture-identity" })
	client.cl = hub.Client()
	offer, err := client.FetchAssignmentOffer(context.Background())
	if err != nil || offer.CheckID != 72 || offer.Epoch != 4 {
		t.Fatalf("offer = %+v, %v", offer, err)
	}
	obs := entities.AgentlessObservation{DedupeKey: "fixture-result", Assignment: &monitoring.CollectorAssignmentGrant{Epoch: 4}}
	if err := client.ReportAssignment(context.Background(), obs); err == nil {
		t.Fatal("different result's ACK accepted")
	}
	exact.Store(true)
	if err := client.ReportAssignment(context.Background(), obs); err != nil {
		t.Fatal(err)
	}
	client.cfg.TLSInsecureSkip = true
	if _, err := client.FetchAssignmentOffer(context.Background()); err == nil || calls.Load() != 3 {
		t.Fatalf("unverified HUB transport attempted: calls=%d err=%v", calls.Load(), err)
	}
}

func TestHUBAssignmentUsesVerifiedDirectAPI(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		if req.URL.Path != relayprobe.AssignmentJobPath || req.Header.Get("Authorization") != "Token fixture-hub" ||
			req.Header.Get("X-Agent-Identity") != "fixture-identity" || req.Header.Get("X-Aiceberg-Hub-Proof") != "" {
			t.Errorf("unexpected direct HUB request: %s", req.URL.Path)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		fmt.Fprint(w, `{"status":"offered","check_id":43,"epoch":4}`)
	}))
	defer upstream.Close()
	t.Setenv("RELAY_SNMP_ASSIGNMENT_ENABLED", "true")
	t.Setenv("RELAY_SNMP_ASSIGNMENT_HUB_RETURN_ENABLED", "")
	cfg := config.Config{HubURL: "https://127.0.0.1:1", APIBaseURL: upstream.URL,
		AgentMode: "hub", Agent: config.AgentCfg{Token: "fixture-hub"}}
	client := NewRelaySNMPProbeClientWithIdentity(cfg, func() string { return "fixture-identity" })
	client.cl = upstream.Client()
	if _, err := client.FetchAssignmentOffer(context.Background()); err == nil || calls.Load() != 0 {
		t.Fatalf("direct HUB assignment defaulted on: calls=%d err=%v", calls.Load(), err)
	}
	t.Setenv("RELAY_SNMP_ASSIGNMENT_HUB_RETURN_ENABLED", "true")
	offer, err := client.FetchAssignmentOffer(context.Background())
	if err != nil || offer.CheckID != 43 || offer.Epoch != 4 || calls.Load() != 1 {
		t.Fatalf("offer=%+v calls=%d err=%v", offer, calls.Load(), err)
	}
	client.cfg.TLSInsecureSkip = true
	if _, err := client.FetchAssignmentOffer(context.Background()); err == nil || calls.Load() != 1 {
		t.Fatalf("unverified HUB→API transport attempted: calls=%d err=%v", calls.Load(), err)
	}
}
