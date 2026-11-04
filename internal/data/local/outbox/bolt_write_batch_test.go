package outbox

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func batchFixture(t *testing.T, maxMB int) (*BoltStore, string) {
	t.Helper()
	path := t.TempDir() + "/synthetic.db"
	s, err := NewBoltStoreWithOptions(path, maxMB, BoltOptions{BatchWrites: true})
	if err != nil {
		t.Fatal(err)
	}
	return s, path
}

func batchBurst(s *BoltStore, n int) error {
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- s.Push(entities.Envelope{ID: fmt.Sprintf("queued-%04d", i), AgentID: "synthetic", AuthHeader: "Token synthetic", IdentityHeader: "synthetic-identity", Endpoint: "/v1/ingest/metrics", Body: map[string]any{"value": strings.Repeat("x", 1024)}})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func TestBoltBatchFailureDoesNotLoseConcurrentSuccessfulWrites(t *testing.T) {
	s, path := batchFixture(t, 1)
	defer s.Close()
	invalid := make(chan error, 1)
	go func() { invalid <- s.Push(entities.Envelope{ID: "oversized", Body: strings.Repeat("x", 2*1024*1024)}) }()
	if err := batchBurst(s, 128); err != nil {
		t.Fatal(err)
	}
	if err := <-invalid; err == nil {
		t.Fatal("oversized write accepted")
	}
	if n, _ := s.Len(); n != 128 {
		t.Fatalf("successful records lost: %d", n)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	old, err := NewBoltStore(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if n, _ := old.Len(); n != 128 {
		t.Fatalf("legacy reopen lost records: %d", n)
	}
}

func TestBoltBatchAcknowledgementWaitsForCommit(t *testing.T) {
	s, _ := batchFixture(t, 1)
	defer s.Close()
	tx, err := s.db.Begin(true)
	if err != nil {
		t.Fatal(err)
	}
	returned := make(chan error, 1)
	go func() { returned <- s.Push(entities.Envelope{ID: "held", Body: "synthetic"}) }()
	select {
	case err := <-returned:
		_ = tx.Rollback()
		t.Fatalf("acknowledged before commit: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-returned:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("commit never returned")
	}
	if n, _ := s.Len(); n != 1 {
		t.Fatal("acknowledged record missing")
	}
}

func TestBoltBatchDuplicateAndAckPreserveCapacity(t *testing.T) {
	s, _ := batchFixture(t, 1)
	defer s.Close()
	e := entities.Envelope{ID: "same", Body: strings.Repeat("x", 700000)}
	if err := s.Push(e); err != nil {
		t.Fatal(err)
	}
	if err := s.Push(e); err != nil {
		t.Fatalf("idempotent overwrite exceeded capacity: %v", err)
	}
	if err := s.Delete([]string{"same", "same", "absent"}); err != nil {
		t.Fatal(err)
	}
	if n, b := s.Len(); n != 0 || b != 0 {
		t.Fatalf("ack state %d/%d", n, b)
	}
}

func TestBoltBatchCrashHelper(t *testing.T) {
	path := os.Getenv("AICEBERG_BOLT_CRASH_PATH")
	if path == "" {
		t.Skip("subprocess-only synthetic writer")
	}
	s, err := NewBoltStoreWithOptions(path, 2, BoltOptions{BatchWrites: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := batchBurst(s, 128); err != nil {
		t.Fatal(err)
	}
	fmt.Println("DURABLE_READY")
	time.Sleep(time.Minute)
}

func killCommittedBatchWriter(t *testing.T, path string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestBoltBatchCrashHelper$")
	cmd.Env = append(os.Environ(), "AICEBERG_BOLT_CRASH_PATH="+path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	ready := make(chan bool, 1)
	go func() {
		s := bufio.NewScanner(stdout)
		for s.Scan() {
			if s.Text() == "DURABLE_READY" {
				ready <- true
				return
			}
		}
		ready <- false
	}()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("child did not confirm commits")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("child timed out")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
}

func TestBoltBatchCommittedRecordsSurviveProcessKill(t *testing.T) {
	path := t.TempDir() + "/synthetic.db"
	killCommittedBatchWriter(t, path)
	old, err := NewBoltStore(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	got, err := old.Peek(200)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]string, 128)
	for i := range want {
		want[i] = fmt.Sprintf("queued-%04d", i)
	}
	for _, env := range got {
		if env.AgentID != "synthetic" || env.AuthHeader != "Token synthetic" || env.IdentityHeader != "synthetic-identity" || env.Endpoint != "/v1/ingest/metrics" {
			t.Fatal("identity/route changed after crash")
		}
		body, ok := env.Body.(map[string]any)
		if !ok || body["value"] != strings.Repeat("x", 1024) {
			t.Fatal("payload changed after crash")
		}
	}
	if !reflect.DeepEqual(envelopeIDs(got), want) {
		t.Fatal("committed records changed after process loss")
	}
}
