# PKG-72 - Benchmark comparavel Datadog

## Ambiente

- Data UTC: 2026-06-19T19:30:00Z
- Responsavel: Codex
- Cliente/lab: lab-controlado-aiceberg
- Host/agente/HUB/relay: agente direto com benchmark gateado por politica
- Versao agente: 0.8.10
- Topologia: direct|hub|relay -> hub -> AIceberg

## Evidencia obrigatoria

- Cenario AIceberg: NOC/SOC contextual, modo soberano/offline, Agent+Agentless e reducao de ruido assistiva.
- Referencia Datadog usada: matriz docs/agente_datadog_paridade.md com capacidade Datadog Agent como referencia de comparacao; sem execucao Datadog em lab nesta evidencia.
- Mesma janela/carga/ambiente ou justificativa: nao ha claim de superioridade porque nao houve execucao Datadog comparavel no mesmo lab.
- Dados brutos rastreaveis: runtime_snapshot_test.go e pkg72_contextual_evidence_homologation.sh validam required_evidence com datadog_reference e claim_allowed=false.
- Revisao operacional: politica exige requires_same_scenario=true, requires_raw_evidence_reference=true e requires_operator_review=true.

## Metricas

- time_to_diagnosis: medido apenas no fluxo AIceberg controlado; nao comparado contra Datadog nesta evidencia.
- deployment_effort: medido apenas como contrato AIceberg; nao comparado contra Datadog nesta evidencia.
- agent_plus_agentless: correlacao funcional validada no painel web e no bundle PKG-69 relay-hub-direct.
- executive_evidence: contextual_evidence exibido no painel web; claim de superioridade permanece bloqueado.

## Resultado

- Status: pass
- Evidencia bruta anexada: internal/bootstrap/runtime_snapshot_test.go TestBuildSuperiorityBenchmarkEvidenceBlocksWeakClaims e docs/agente_datadog_paridade.md.
- Observacoes: o resultado aprovado e a trava de governanca, nao uma declaracao de que AIceberg e melhor que Datadog.
- Rollback validado: manter claim_allowed=false bloqueia qualquer texto de superioridade mesmo com diferenciais funcionais.
- Revisor: Codex
- Aprovacao fechamento: yes
