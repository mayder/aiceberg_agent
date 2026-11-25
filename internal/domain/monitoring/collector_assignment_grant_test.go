package monitoring

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

func TestCollectorAssignmentGrantRejectsChangedJobScopeAndLease(t *testing.T) {
	now := time.UnixMilli(1000)
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	public := key.Public().(ed25519.PublicKey)
	job := []byte(`{"check_id":43,"tipo":"snmp"}`)
	digest := sha256.Sum256(job)
	msg := fmt.Sprintf("aiceberg.monitoring.collector-assignment.v1\n7\n43\n10\n11\n2\n61000\n%x\n", digest)
	grant := CollectorAssignmentGrant{
		Version: 1, ClientID: 7, CheckID: 43, HubID: 10, OwnerID: 11, Epoch: 2, ExpiresAtMS: 61000,
		JobSHA256: hex.EncodeToString(digest[:]), Signature: hex.EncodeToString(ed25519.Sign(key, []byte(msg))),
	}
	scope := CollectorAssignmentScope{ClientID: 7, CheckID: 43, HubID: 10, OwnerID: 11, Epoch: 2}
	if err := grant.Verify(scope, job, public, now); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		edit func(*CollectorAssignmentGrant, *CollectorAssignmentScope)
	}{
		{"legacy envelope", func(g *CollectorAssignmentGrant, _ *CollectorAssignmentScope) { g.Version = 0 }},
		{"tenant", func(_ *CollectorAssignmentGrant, s *CollectorAssignmentScope) { s.ClientID++ }},
		{"previous epoch", func(_ *CollectorAssignmentGrant, s *CollectorAssignmentScope) { s.Epoch++ }},
		{"relay", func(_ *CollectorAssignmentGrant, s *CollectorAssignmentScope) { s.OwnerID++ }},
		{"hub", func(_ *CollectorAssignmentGrant, s *CollectorAssignmentScope) { s.HubID++ }},
		{"lease extension", func(g *CollectorAssignmentGrant, _ *CollectorAssignmentScope) { g.ExpiresAtMS++ }},
		{"broken signature", func(g *CollectorAssignmentGrant, _ *CollectorAssignmentScope) { g.Signature = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changedGrant, changedScope := grant, scope
			test.edit(&changedGrant, &changedScope)
			if changedGrant.Verify(changedScope, job, public, now) == nil {
				t.Fatal("accepted changed assignment")
			}
		})
	}
	if grant.Verify(scope, []byte(`{"check_id":42,"tipo":"snmp"}`), public, now) == nil ||
		grant.Verify(scope, job, public, time.UnixMilli(61000)) == nil {
		t.Fatal("accepted altered job or expired lease")
	}
}
