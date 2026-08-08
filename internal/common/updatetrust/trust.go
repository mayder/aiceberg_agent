package updatetrust

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	Algorithm            = "ed25519-sha256"
	MessagePrefix        = "aiceberg-agent-update:v1"
	OfficialKeyID        = "aiceberg-agent-prod-v1"
	OfficialPublicKeyHex = "9a74529340757946a091dff57d1d1fa721d4b129d76748b6e1c4533514c54045"
)

func Message(targetVersion, artifactSHA256, signingKeyID string) string {
	return strings.Join([]string{
		MessagePrefix,
		strings.TrimSpace(targetVersion),
		normalizeSHA256(artifactSHA256),
		strings.TrimSpace(signingKeyID),
	}, "\n")
}

func Verify(publicKeyValue, targetVersion, artifactSHA256, algorithm, signatureValue, signingKeyID string) error {
	publicKey, err := DecodePublicKey(publicKeyValue)
	if err != nil {
		return fmt.Errorf("artifact trust public key invalid: %w", err)
	}
	signature, err := Decode(signatureValue)
	if err != nil {
		return fmt.Errorf("artifact signature invalid: %w", err)
	}
	if strings.TrimSpace(algorithm) == "" {
		algorithm = Algorithm
	}
	if !strings.EqualFold(strings.TrimSpace(algorithm), Algorithm) {
		return fmt.Errorf("artifact signature algorithm unsupported: %s", algorithm)
	}
	if normalizeSHA256(artifactSHA256) == "" {
		return errors.New("artifact sha256 missing for trust validation")
	}
	message := Message(targetVersion, artifactSHA256, signingKeyID)
	if !ed25519.Verify(publicKey, []byte(message), signature) {
		return errors.New("artifact signature verification failed")
	}
	return nil
}

func DecodePublicKey(value string) (ed25519.PublicKey, error) {
	raw, err := Decode(value)
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("expected %d bytes, got %d", ed25519.PublicKeySize, len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

func Decode(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("empty value")
	}
	if raw, err := hex.DecodeString(strings.TrimPrefix(value, "hex:")); err == nil {
		return raw, nil
	}
	if raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "base64:")); err == nil {
		return raw, nil
	}
	return nil, errors.New("expected hex or base64")
}

func normalizeSHA256(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.TrimPrefix(value, "sha256:")
}
