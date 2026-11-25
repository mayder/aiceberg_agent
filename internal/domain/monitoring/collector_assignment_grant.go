package monitoring

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

var ErrCollectorAssignmentInvalid = errors.New("invalid collector assignment grant")

// CollectorAssignmentGrant is the coordinator's signed authority for one check job.
type CollectorAssignmentGrant struct {
	Version     int    `json:"version"`
	ClientID    int64  `json:"client_id"`
	CheckID     int64  `json:"check_id"`
	HubID       int64  `json:"hub_id"`
	OwnerID     int64  `json:"owner_id"`
	Epoch       uint64 `json:"epoch"`
	ExpiresAtMS int64  `json:"expires_at_ms"`
	JobSHA256   string `json:"job_sha256"`
	Signature   string `json:"signature"`
}

func (g CollectorAssignmentGrant) signedMessage() []byte {
	return []byte(fmt.Sprintf("aiceberg.monitoring.collector-assignment.v%d\n%d\n%d\n%d\n%d\n%d\n%d\n%s\n",
		g.Version, g.ClientID, g.CheckID, g.HubID, g.OwnerID, g.Epoch, g.ExpiresAtMS, g.JobSHA256))
}

type CollectorAssignmentScope struct {
	ClientID int64
	CheckID  int64
	HubID    int64
	OwnerID  int64
	Epoch    uint64
}

// Verify checks the exact received job wire and the current authorized scope.
// The caller must obtain the expected epoch from its acknowledged assignment,
// and pin the coordinator's dedicated public key independently of the grant.
func (g CollectorAssignmentGrant) Verify(scope CollectorAssignmentScope, job []byte, key ed25519.PublicKey, now time.Time) error {
	if g.Version != 1 || scope.ClientID <= 0 || scope.CheckID <= 0 || scope.HubID <= 0 ||
		scope.OwnerID <= 0 || scope.Epoch == 0 || g.ClientID != scope.ClientID || g.CheckID != scope.CheckID ||
		g.HubID != scope.HubID || g.OwnerID != scope.OwnerID || g.Epoch != scope.Epoch ||
		now.UnixMilli() <= 0 || g.ExpiresAtMS <= now.UnixMilli() || len(job) == 0 ||
		len(key) != ed25519.PublicKeySize || len(g.Signature) != 2*ed25519.SignatureSize {
		return ErrCollectorAssignmentInvalid
	}
	digest := sha256.Sum256(job)
	if g.JobSHA256 != hex.EncodeToString(digest[:]) {
		return ErrCollectorAssignmentInvalid
	}
	signature, err := hex.DecodeString(g.Signature)
	if err != nil || !ed25519.Verify(key, g.signedMessage(), signature) {
		return ErrCollectorAssignmentInvalid
	}
	return nil
}
