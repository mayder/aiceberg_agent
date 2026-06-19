
## PKG-72 contextual evidence homologation

- repo: info — /Users/brenomayder/projects/desktop/aiceberg_agent
- os: info — Darwin 25.5.0 arm64
- go: info — go version go1.25.4 darwin/arm64

## focused local validation

- go-test-focused: pass — log=docs/evidence/pkg72/contextual-ai-offline-20260619T193000Z/raw/go-test-focused.log
- contextual-evidence: pass — snapshot exposes contextual_evidence, local AI guardrails, privacy, offline-first and benchmark gate
- relay-topology: pass — tests keep relay_to_hub_only=true and direct_api_from_relay=false in relay mode
- offline-replay-local: pass — BoltStore keeps envelopes until ACK and ACK is idempotent
- superiority-claim: pass — claim_allowed=false and benchmark status remains pending_evidence

## required real evidence

- noc-soc-incident-host-agentless: evidence — path=docs/evidence/pkg72/contextual-ai-offline-20260619T193000Z/inputs/noc_soc_incident_host_agentless.md sha256=947b7a09bd7a5fba38153d241dfa27730cb8d594798f62bf10b00fd4d126a327 bytes=1891
- offline-replay-24h: evidence — path=docs/evidence/pkg72/contextual-ai-offline-20260619T193000Z/inputs/offline_replay_24h.md sha256=6cac07f88655260059000682b4c42f93bbf17a26e9ae62f1acbec917e2234113 bytes=1753
- regulated-client-minimal-collection: evidence — path=docs/evidence/pkg72/contextual-ai-offline-20260619T193000Z/inputs/regulated_client_minimal_collection.md sha256=46d79d38672deec7f1b1326ee345c15d9536946e8ad55342cf5c3cd508e0d7b7 bytes=1466
- noise-cost-before-after: evidence — path=docs/evidence/pkg72/contextual-ai-offline-20260619T193000Z/inputs/noise_cost_before_after.md sha256=04496cad874a71ec15be797f340bc00518da27804b174226a32ffa023e987541 bytes=1574
- datadog-scenario-benchmark: evidence — path=docs/evidence/pkg72/contextual-ai-offline-20260619T193000Z/inputs/datadog_scenario_benchmark.md sha256=dac751044cb96878371bbd04e4318d12839429c841bf6225ef7a417fdffbfc92 bytes=1965

## benchmark scenarios

- noc_soc_context: evidence — time_to_diagnosis, evidence_completeness and operator_steps covered by attached incident evidence
- sovereign_offline: evidence — offline_replay_success, duplicate_rate and support_export_integrity covered by attached replay evidence
- agent_plus_agentless: evidence — correlation_detected and agentless_observation_link covered by attached incident evidence
- noise_reduction: evidence — noise_before, noise_after and manual_review_required covered by attached noise/cost evidence
- datadog_superiority_claim: blocked — attached benchmark evidence keeps claim_allowed=false until a scenario-matched Datadog run exists

## closure rule

- real-evidence-manifest: ready-for-review — 5/5 files present with SHA256; explicit closure acceptance still required
- pkg72-status: accepted-for-closure — all required real evidence is present and PKG72_ACCEPT_CLOSURE=true
- evidence-file: written — docs/evidence/pkg72/contextual-ai-offline-20260619T193000Z/evidence.md
- evidence-manifest-tsv: written — docs/evidence/pkg72/contextual-ai-offline-20260619T193000Z/MANIFEST.tsv
