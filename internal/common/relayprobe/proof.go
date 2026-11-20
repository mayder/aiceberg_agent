package relayprobe

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const JobPath = "/v1/relay-snmp-probe/job"
const ResultPath = "/v1/relay-snmp-probe/result"

func Proof(secret, authorization, path string, body []byte, timestamp string) string {
	canonical := fmt.Sprintf("relay-snmp-probe.v1\nPOST\n%s\n%s\n%x\n%x", path, timestamp, sha256.Sum256([]byte(authorization)), sha256.Sum256(body))
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}
