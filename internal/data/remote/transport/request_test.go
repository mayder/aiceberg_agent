package transport

import (
	"bytes"
	"compress/gzip"
	"io"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestBuildRequest_IdempotencyAndGzip(t *testing.T) {
	cfg := config.Config{
		HTTPGzip:        true,
		HTTPIdempotency: true,
	}
	batch := []entities.Envelope{{ID: "1"}}
	req, err := buildRequest("http://example", batch, cfg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if ct := req.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected content-type json, got %q", ct)
	}
	if ce := req.Header.Get("Content-Encoding"); ce != "gzip" {
		t.Fatalf("expected gzip encoding, got %q", ce)
	}
	if req.Header.Get("Idempotency-Key") == "" {
		t.Fatalf("expected idempotency key")
	}
	body, _ := io.ReadAll(req.Body)
	rdr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("expected gzip body, got %v", err)
	}
	defer rdr.Close()
	unzipped, err := io.ReadAll(rdr)
	if err != nil {
		t.Fatalf("failed to read gzip body: %v", err)
	}
	if !bytes.Contains(unzipped, []byte(`"envelope_id":"1"`)) {
		t.Fatalf("unexpected payload: %s", string(unzipped))
	}
}

func TestBuildRequest_NoIdempotency(t *testing.T) {
	cfg := config.Config{
		HTTPGzip:        false,
		HTTPIdempotency: false,
	}
	batch := []entities.Envelope{{ID: "1"}}
	req, err := buildRequest("http://example", batch, cfg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if req.Header.Get("Idempotency-Key") != "" {
		t.Fatalf("did not expect idempotency key")
	}
}

func TestSetEnvelopeIdentityHeader(t *testing.T) {
	req, err := buildRequest("http://example", []entities.Envelope{{ID: "1"}}, config.Config{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	setEnvelopeIdentityHeader(req, []entities.Envelope{
		{ID: "1"},
		{ID: "2", IdentityHeader: "identity-claim"},
	})
	if got := req.Header.Get("X-Agent-Identity"); got != "identity-claim" {
		t.Fatalf("expected identity header, got %q", got)
	}
}
