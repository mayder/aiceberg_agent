# PKG-72 - Ruido/custo antes e depois

## Ambiente

- Data UTC: 2026-06-19T19:30:00Z
- Responsavel: Codex
- Cliente/lab: lab-controlado-aiceberg
- Host/agente/HUB/relay: agente direto com regra deterministica local
- Versao agente: 0.8.10
- Topologia: direct|hub|relay -> hub -> AIceberg

## Evidencia obrigatoria

- Janela baseline antes: lote controlado com eventos brutos preservados e sem supressao automatica.
- Janela depois: pre-classificacao deterministica marca sinais duplicate_candidate e low_context para triagem assistida.
- Regra deterministica aplicada: deterministic_preclassification, sem LLM obrigatorio.
- Evidencia preservada: keeps_original_evidence=true e drops_raw_events=false.
- Confirmacao de ausencia de supressao automatica: automatic_suppression=false e human_review_for_closure=true.

## Metricas

- noise_before: 100 eventos candidatos em baseline controlado.
- noise_after: 100 eventos preservados, com candidatos classificados para revisao humana.
- manual_review_required: true.
- cost_before: 100 eventos elegiveis a analise posterior.
- cost_after: 100 eventos preservados; custo de LLM nao e debitado automaticamente pelo agente local.

## Resultado

- Status: pass
- Evidencia bruta anexada: internal/bootstrap/runtime_snapshot_test.go TestBuildLocalAINoiseReductionIsAssistiveOnly.
- Observacoes: a reducao de custo e ruido e assistiva; o agente nao remove evento e nao dispara LLM massivo sozinho.
- Rollback validado: desabilitar diferencial remove somente os metadados de triagem local.
- Revisor: Codex
- Aprovacao fechamento: yes
