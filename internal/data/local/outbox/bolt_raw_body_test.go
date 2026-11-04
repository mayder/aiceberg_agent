package outbox

import (
	"encoding/json"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"testing"
)

func TestBoltRawBodyIsOwnedAndLegacyReopenPreservesContent(t *testing.T) {
	path := t.TempDir() + "/synthetic.db"
	s, err := NewBoltStoreWithOptions(path, 2, BoltOptions{RawBodies: true, BatchWrites: true, RebuildFreelist: true})
	if err != nil {
		t.Fatal(err)
	}
	body := json.RawMessage(`{"value":42,"samples":[1,2,3]}`)
	e := entities.Envelope{ID: "retained", AgentID: "relay", AuthHeader: "Token synthetic", IdentityHeader: "identity", Endpoint: "/v1/logs/raw", Body: body}
	if err := s.Push(e); err != nil {
		t.Fatal(err)
	}
	got, err := s.Peek(1)
	if err != nil || len(got) != 1 {
		t.Fatal(err)
	}
	raw, ok := got[0].Body.(json.RawMessage)
	if !ok || string(raw) != string(body) {
		t.Fatal("not raw JSON")
	}
	e.Body = json.RawMessage(`{"value":99}`)
	if err := s.Push(e); err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(body) {
		t.Fatal("body aliases transaction memory")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	old, err := NewBoltStore(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	records, err := old.Peek(2)
	if err != nil || len(records) != 1 {
		t.Fatal(err)
	}
	r := records[0]
	if r.AuthHeader != e.AuthHeader || r.IdentityHeader != e.IdentityHeader || r.Endpoint != e.Endpoint || r.AgentID != e.AgentID {
		t.Fatal("identity or route changed")
	}
	if r.Body.(map[string]any)["value"] != float64(99) {
		t.Fatal("legacy reopen changed body")
	}
}
