package usecase

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

const rawLogsEndpoint = "/v1/logs/raw"

func (uc *FlushOutbox) normalizeOversizedLogEnvelopes(batch []entities.Envelope) (int, error) {
	replacer, ok := uc.outbox.(ports.OutboxEnvelopeReplacer)
	normalized := 0
	for _, envelope := range batch {
		oversized, err := envelopeExceedsRequestLimit(envelope, uc.maxBatchBytes)
		if err != nil {
			return normalized, err
		}
		if !oversized || strings.TrimSpace(envelope.Endpoint) != rawLogsEndpoint {
			continue
		}
		if !ok {
			return normalized, errors.New("outbox does not support atomic split of oversized log envelope")
		}
		parts, err := splitRawLogEnvelope(envelope, uc.maxBatchBytes)
		if err != nil {
			uc.log.Error(logger.KV("oversized log envelope retained",
				"route", envelope.Endpoint,
				"envelope_id", envelope.ID,
				"max_batch_bytes", uc.maxBatchBytes,
				"err", err,
			))
			return normalized, err
		}
		if err := replacer.ReplaceEnvelope(envelope.ID, parts); err != nil {
			return normalized, fmt.Errorf("replace oversized log envelope %q: %w", envelope.ID, err)
		}
		normalized++
		uc.log.Info(logger.KV("oversized log envelope split",
			"route", envelope.Endpoint,
			"envelope_id", envelope.ID,
			"parts", len(parts),
			"max_batch_bytes", uc.maxBatchBytes,
		))
	}
	return normalized, nil
}

func envelopeExceedsRequestLimit(envelope entities.Envelope, maxBytes int) (bool, error) {
	raw, err := json.Marshal(envelope)
	if err != nil {
		return false, fmt.Errorf("marshal envelope %q: %w", envelope.ID, err)
	}
	return len(raw)+2 > maxBytes, nil
}

func splitRawLogEnvelope(envelope entities.Envelope, maxBytes int) ([]entities.Envelope, error) {
	body, events, err := decodeRawLogBody(envelope.Body)
	if err != nil {
		return nil, fmt.Errorf("decode envelope %q body: %w", envelope.ID, err)
	}
	if len(events) < 2 {
		return nil, fmt.Errorf("envelope %q has no splittable log events", envelope.ID)
	}

	parts := make([]entities.Envelope, 0, 2)
	for start, part := 0, 0; start < len(events); part++ {
		candidate := envelope
		candidate.ID = deterministicSplitEnvelopeID(envelope.ID, part)
		baseBytes, err := rawLogEnvelopeBaseRequestBytes(candidate, body)
		if err != nil {
			return nil, err
		}
		end, currentBytes := start, baseBytes
		for end < len(events) {
			nextBytes := len(events[end])
			if end > start {
				nextBytes++
			}
			if currentBytes+nextBytes > maxBytes {
				break
			}
			currentBytes += nextBytes
			end++
		}
		if end == start {
			return nil, fmt.Errorf("event %d in envelope %q exceeds request limit %d", start, envelope.ID, maxBytes)
		}
		candidate.Body, err = bodyWithEvents(body, events[start:end])
		if err != nil {
			return nil, err
		}
		parts = append(parts, candidate)
		start = end
	}
	return parts, nil
}

func decodeRawLogBody(value any) (map[string]json.RawMessage, []json.RawMessage, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, nil, err
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, nil, err
	}
	eventsRaw, ok := body["events"]
	if !ok {
		return nil, nil, errors.New("events field is missing")
	}
	var events []json.RawMessage
	if err := json.Unmarshal(eventsRaw, &events); err != nil {
		return nil, nil, fmt.Errorf("decode events: %w", err)
	}
	return body, events, nil
}

func rawLogEnvelopeBaseRequestBytes(envelope entities.Envelope, body map[string]json.RawMessage) (int, error) {
	emptyBody, err := bodyWithEvents(body, nil)
	if err != nil {
		return 0, err
	}
	envelope.Body = emptyBody
	raw, err := json.Marshal(envelope)
	if err != nil {
		return 0, err
	}
	return len(raw) + 2, nil
}

func bodyWithEvents(body map[string]json.RawMessage, events []json.RawMessage) (json.RawMessage, error) {
	copyBody := make(map[string]json.RawMessage, len(body))
	for key, value := range body {
		copyBody[key] = value
	}
	if events == nil {
		events = []json.RawMessage{}
	}
	rawEvents, err := json.Marshal(events)
	if err != nil {
		return nil, err
	}
	copyBody["events"] = rawEvents
	return json.Marshal(copyBody)
}

func deterministicSplitEnvelopeID(originalID string, part int) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d", originalID, part)))
	raw := hex.EncodeToString(digest[:16])
	return raw[0:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:32]
}
