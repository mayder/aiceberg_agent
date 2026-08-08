package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateSignVerifyAndRejectTamperedArtifact(t *testing.T) {
	dir := t.TempDir()
	privatePath := filepath.Join(dir, "private.pem")
	publicPath := filepath.Join(dir, "public.pem")
	artifactPath := filepath.Join(dir, "agent.zip")
	manifestPath := filepath.Join(dir, "UPDATE_SIGNATURES.json")

	if err := generate([]string{"-private-key", privatePath, "-public-key", publicPath}); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if err := os.WriteFile(artifactPath, []byte("official artifact"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sign([]string{
		"-private-key", privatePath,
		"-version", "0.8.47",
		"-key-id", "test-key-v1",
		"-output", manifestPath,
		"-artifact", artifactPath,
	}); err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := verify([]string{"-manifest", manifestPath, "-public-key", publicPath, "-artifact-dir", dir}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := os.WriteFile(artifactPath, []byte("tampered artifact"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verify([]string{"-manifest", manifestPath, "-public-key", publicPath, "-artifact-dir", dir}); err == nil {
		t.Fatal("expected tampered artifact rejection")
	}
}

func TestGenerateRefusesToOverwriteExistingPrivateKey(t *testing.T) {
	dir := t.TempDir()
	privatePath := filepath.Join(dir, "private.pem")
	publicPath := filepath.Join(dir, "public.pem")
	if err := os.WriteFile(privatePath, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := generate([]string{"-private-key", privatePath, "-public-key", publicPath}); err == nil {
		t.Fatal("expected overwrite refusal")
	}
	raw, err := os.ReadFile(privatePath)
	if err != nil || string(raw) != "keep" {
		t.Fatalf("existing private key changed: %q err=%v", raw, err)
	}
}
