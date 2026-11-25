package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/you/aiceberg_agent/internal/common/relayprobe"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func (c *RelaySNMPProbeClient) FetchAssignmentOffer(ctx context.Context) (entities.RelaySNMPOffer, error) {
	var offer entities.RelaySNMPOffer
	err := c.exchange(ctx, relayprobe.AssignmentJobPath, map[string]string{"operation": "offer"}, &offer)
	return offer, err
}

func (c *RelaySNMPProbeClient) ClaimAssignment(ctx context.Context, offer entities.RelaySNMPOffer) (entities.RelaySNMPAssignment, error) {
	var assignment entities.RelaySNMPAssignment
	request := map[string]any{"operation": "claim", "check_id": offer.CheckID, "epoch": offer.Epoch,
		"credential_mode": entities.AgentlessCredentialReferenceV1}
	err := c.exchange(ctx, relayprobe.AssignmentJobPath, request, &assignment)
	return assignment, err
}

func (c *RelaySNMPProbeClient) ResolveAssignmentCredentials(ctx context.Context, job entities.AgentlessJob) (*entities.AgentlessSnmpProfile, error) {
	if job.CommandID == "" || job.SNMP == nil || job.Endpoint == nil || job.ProbePolicy == nil ||
		job.CredentialDelivery != entities.AgentlessCredentialReferenceV1 {
		return nil, errSNMPCredentialUnavailable
	}
	lookup := snmpCredentialLookup(job)
	var raw json.RawMessage
	if err := c.exchange(ctx, relayprobe.AssignmentJobPath, lookup, &raw); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errSNMPCredentialUnavailable
	}
	return decodeSNMPCredential(bytes.NewReader(raw), *job.SNMP, lookup.CredentialRef, lookup.CommandID)
}

func (c *RelaySNMPProbeClient) ReportAssignment(ctx context.Context, observation entities.AgentlessObservation) error {
	if observation.Assignment == nil || observation.DedupeKey == "" {
		return errors.New("relay_assignment_result_invalid")
	}
	var receipt struct {
		Status    string `json:"status"`
		Accepted  bool   `json:"accepted"`
		DedupeKey string `json:"dedupe_key"`
	}
	err := c.exchange(ctx, relayprobe.AssignmentResultPath, map[string]any{"observation": observation}, &receipt)
	if errors.Is(err, errRelayAssignmentConflict) && receipt.Status == "fenced" && receipt.DedupeKey == observation.DedupeKey {
		return entities.ErrRelayAssignmentFenced
	}
	if err != nil {
		return err
	}
	if receipt.Status != "ok" || !receipt.Accepted || receipt.DedupeKey != observation.DedupeKey {
		return errors.New("relay_assignment_result_unacknowledged")
	}
	return nil
}
