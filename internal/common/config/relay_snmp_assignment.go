package config

import (
	"crypto/ed25519"
	"encoding/hex"
	"os"
)

type RelaySNMPAssignmentSettings struct {
	HubID     int64
	PublicKey ed25519.PublicKey
}

func (c Config) RelaySNMPAssignmentEnabled() bool {
	return boolEnv("RELAY_SNMP_ASSIGNMENT_ENABLED", false)
}

func (c Config) RelaySNMPAssignmentSettings() (RelaySNMPAssignmentSettings, bool) {
	if !c.RelaySNMPAssignmentEnabled() || c.AgentID <= 0 || c.AgentClientID <= 0 {
		return RelaySNMPAssignmentSettings{}, false
	}
	hub := intEnv("RELAY_SNMP_ASSIGNMENT_HUB_ID", 0)
	switch c.Mode() {
	case "relay":
	case "hub":
		if !c.RelaySNMPHubReturnEnabled() {
			return RelaySNMPAssignmentSettings{}, false
		}
		hub = c.AgentID
	default:
		return RelaySNMPAssignmentSettings{}, false
	}
	key, err := hex.DecodeString(os.Getenv("RELAY_SNMP_ASSIGNMENT_PUBLIC_KEY"))
	if hub <= 0 || err != nil || len(key) != ed25519.PublicKeySize {
		return RelaySNMPAssignmentSettings{}, false
	}
	return RelaySNMPAssignmentSettings{HubID: int64(hub), PublicKey: ed25519.PublicKey(key)}, true
}

func (c Config) RelaySNMPHubReturnEnabled() bool {
	return c.Mode() == "hub" && boolEnv("RELAY_SNMP_ASSIGNMENT_HUB_RETURN_ENABLED", false)
}
