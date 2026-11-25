package entities

import (
	"encoding/json"
	"errors"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

var ErrRelayAssignmentFenced = errors.New("relay assignment result fenced")

type RelaySNMPOffer struct {
	Status  string `json:"status"`
	CheckID int64  `json:"check_id"`
	Epoch   uint64 `json:"epoch"`
}

type RelaySNMPAssignment struct {
	Status string                              `json:"status"`
	Grant  monitoring.CollectorAssignmentGrant `json:"grant"`
	Job    json.RawMessage                     `json:"job"`
}
