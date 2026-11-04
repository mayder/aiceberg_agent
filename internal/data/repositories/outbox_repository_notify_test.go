package repositories

import (
	"errors"
	"testing"

	"github.com/you/aiceberg_agent/internal/data/local/outbox"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type notifyStore struct {
	*outbox.MemStore
	fail bool
}

func (s *notifyStore) Push(e entities.Envelope) error {
	if s.fail {
		return errors.New("controlled storage failure")
	}
	return s.MemStore.Push(e)
}

func TestOutboxNotifiesOnlyAfterSuccessfulPersistence(t *testing.T) {
	s := &notifyStore{MemStore: outbox.NewMemStore()}
	notified := 0
	r := NewOutboxRepositoryWithNotify(s, func() {
		if count, _ := s.Len(); count != 1 {
			t.Fatal("wake emitted before storage accepted data")
		}
		notified++
	})
	if err := r.Append(entities.Envelope{ID: "persisted"}); err != nil || notified != 1 {
		t.Fatal("successful append did not notify")
	}
	s.fail = true
	if err := r.Append(entities.Envelope{ID: "not-persisted"}); err == nil || notified != 1 {
		t.Fatal("failed append emitted a wake or reported success")
	}
	if err := r.Ack([]string{"persisted"}); err != nil || notified != 1 {
		t.Fatal("ACK emitted an arrival wake")
	}
}
