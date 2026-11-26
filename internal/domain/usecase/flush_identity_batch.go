package usecase

import (
	"encoding/json"
	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type verifiedBatchKey struct {
	group    flushGroupKey
	verified bool
}

type verifiedBatchIdentity struct {
	header string
	issued int64
	valid  bool
	scope  string
	reason string
}

// Selection preserves the input order and uses an original, verified header.
// Neither durable envelopes nor the original measurement timestamps are rewritten.
func (uc *FlushOutbox) groupVerifiedIdentities(batch []entities.Envelope) map[flushGroupKey][]entities.Envelope {
	groups := make(map[verifiedBatchKey][]entities.Envelope)
	choices := make(map[verifiedBatchKey]verifiedBatchIdentity)
	cache := make(map[flushGroupKey]verifiedBatchIdentity)
	now := uc.now().Unix()
	reasons := make(map[string]int)
	for _, envelope := range batch {
		original := flushGroupKey{auth: envelope.AuthHeader, identity: envelope.IdentityHeader, endpoint: envelope.Endpoint}
		cacheKey := flushGroupKey{auth: original.auth, identity: original.identity}
		identity, found := cache[cacheKey]
		if !found {
			identity.scope, identity.issued, identity.reason = config.IdentityBatchScopeEvidence(original.identity, original.auth, now)
			identity.valid = identity.reason == "verified"
			identity.header = original.identity
			cache[cacheKey] = identity
		}
		reasons[identity.reason]++
		key := verifiedBatchKey{group: original, verified: identity.valid}
		if identity.valid {
			key.group.identity = identity.scope
			if previous, ok := choices[key]; !ok || identity.issued > previous.issued {
				choices[key] = identity
			}
		}
		groups[key] = append(groups[key], envelope)
	}
	result := make(map[flushGroupKey][]entities.Envelope, len(groups))
	for key, envelopes := range groups {
		group := key.group
		if key.verified {
			group.verifiedScope = key.group.identity
			group.identity = choices[key].header
			for i := range envelopes {
				envelopes[i].IdentityHeader = group.identity
			}
		}
		result[group] = envelopes
	}
	uc.mu.Lock()
	uc.snapshot.LastIdentityReasons = reasons
	uc.mu.Unlock()
	return result
}

func flushBackoffKey(group flushGroupKey) string {
	if group.verifiedScope != "" {
		key, _ := json.Marshal([]string{group.auth, group.verifiedScope, group.endpoint})
		return "verified:" + string(key)
	}
	return group.auth + "|" + group.identity + "|" + group.endpoint
}
