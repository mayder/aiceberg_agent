package relayprobe

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const JobPath = "/v1/relay-snmp-probe/job"
const ResultPath = "/v1/relay-snmp-probe/result"
const AssignmentJobPath = "/v1/relay-snmp-assignment/job"
const AssignmentResultPath = "/v1/relay-snmp-assignment/result"

func IsAssignmentPath(path string) bool {
	return path == AssignmentJobPath || path == AssignmentResultPath
}

func BodyLimit(path string) int64 {
	if IsAssignmentPath(path) {
		return 512 * 1024
	}
	return 16384
}

func Proof(secret, authorization, path string, body []byte, timestamp string) string {
	canonical := fmt.Sprintf("relay-snmp-probe.v1\nPOST\n%s\n%s\n%x\n%x", path, timestamp, sha256.Sum256([]byte(authorization)), sha256.Sum256(body))
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}
