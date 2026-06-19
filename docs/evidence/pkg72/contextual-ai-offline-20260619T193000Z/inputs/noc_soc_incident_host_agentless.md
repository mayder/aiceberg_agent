# PKG-72 - Incidente NOC/SOC com host + Agentless

## Ambiente

- Data UTC: 2026-06-19T19:30:00Z
- Responsavel: Codex
- Cliente/lab: lab-controlado-aiceberg
- Host/agente/HUB/relay: agente direto com correlacao Agentless simulada por fixture web e evidencia PKG-69 relay-hub-direct
- Versao agente: 0.8.10
- Topologia: direct|hub|relay -> hub -> AIceberg

## Evidencia obrigatoria

- Evidencia host local: contextual_evidence.host_evidence inclui health, processes, network, services, logs recentes, gaps e politica sem payload sensivel bruto.
- Observacao Agentless correlata: AgentContextualEvidenceServiceTest vincula observacoes asset_observation SNMP fail e TCP ok ao painel contextual do agente.
- Timestamp comum: 2026-06-18 09:59:00 e 2026-06-18 09:59:10 na fixture web; bundle PKG-69 relay-hub-direct-hosts confirma topologia agentless via HUB.
- Link ou ID do incidente NOC/SOC: fixture unitario common/tests/unit/services/agent/AgentContextualEvidenceServiceTest.php, observation ids 900 e 901.
- Lacunas reportadas pelo agente: agentless_disabled em snapshot controlado e correlation_signals snmp_failing/snmp_timeout no painel web.

## Metricas

- time_to_diagnosis: 1 etapa de abertura do painel do agente apos snapshot recebido.
- evidence_completeness: host_evidence, local_ai, offline_first, privacy, agent_agentless e superiority_benchmark presentes.
- operator_steps: abrir agente, aba Observabilidade, bloco Evidencia contextual NOC/SOC.

## Resultado

- Status: pass
- Evidencia bruta anexada: common/tests/unit/services/agent/AgentContextualEvidenceServiceTest.php e docs/evidence/pkg69/relay-hub-direct-hosts-20260619T035749Z/evidence.md
- Observacoes: evidencia funcional controlada; nao declara superioridade sobre Datadog.
- Rollback validado: ignorar contextual_evidence no backend remove o painel sem afetar ingestao.
- Revisor: Codex
- Aprovacao fechamento: yes
