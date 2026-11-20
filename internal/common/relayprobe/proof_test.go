package relayprobe

import "testing"

func TestProofMatchesPHPContractVector(t *testing.T) {
	got := Proof("fixture-hub-key", "Token fixture-relay-key", JobPath, []byte(`{"command_id":"fixture-command-0001"}`), "1805716800")
	if got != "3252da886d680b689c7554b6dff2c489ec7f360a89cf85e0aba341b600be6f3a" {
		t.Fatal("HUB proof contract mismatch")
	}
	if got == Proof("fixture-hub-key", "Token other-relay", JobPath, []byte(`{"command_id":"fixture-command-0001"}`), "1805716800") {
		t.Fatal("proof does not bind relay identity")
	}
}
