package updatetrust

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestVerifyAcceptsValidAndRejectsTamperedArtifact(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("official artifact"))
	sha := hex.EncodeToString(sum[:])
	signature := ed25519.Sign(privateKey, []byte(Message("0.8.47", sha, OfficialKeyID)))

	if err := Verify(hex.EncodeToString(publicKey), "0.8.47", sha, Algorithm, hex.EncodeToString(signature), OfficialKeyID); err != nil {
		t.Fatalf("expected valid signature: %v", err)
	}
	other := sha256.Sum256([]byte("tampered artifact"))
	err = Verify(hex.EncodeToString(publicKey), "0.8.47", hex.EncodeToString(other[:]), Algorithm, hex.EncodeToString(signature), OfficialKeyID)
	if err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("expected tampered artifact rejection, got %v", err)
	}
}

func TestOfficialPublicKeyIsValid(t *testing.T) {
	if _, err := DecodePublicKey(OfficialPublicKeyHex); err != nil {
		t.Fatalf("official public key must be valid: %v", err)
	}
}
