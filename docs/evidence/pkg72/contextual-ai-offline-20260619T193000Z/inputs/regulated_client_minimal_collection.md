# PKG-72 - Cliente regulado com coleta reduzida

## Ambiente

- Data UTC: 2026-06-19T19:30:00Z
- Responsavel: Codex
- Cliente/lab: lab-controlado-aiceberg
- Host/agente/HUB/relay: agente direto com PRIVACY_PROFILE=minimal e SENSITIVE_MODE=true
- Versao agente: 0.8.10
- Topologia: direct|hub|relay -> hub -> AIceberg

## Evidencia obrigatoria

- Perfil de privacidade aplicado: minimal.
- Coletores minimizados: logs, processes, services, network, inventory e oslog_diag desabilitados na fixture.
- Campos mascarados/hash: APIKey e HubToken de fixture sensivel testados e ausentes no JSON final.
- Confirmacao de ausencia de segredo bruto: teste falha se os valores sensiveis de fixture aparecerem na evidencia contextual serializada.
- Evidencia de rollback da configuracao: remover PRIVACY_PROFILE=minimal volta ao perfil sensivel padrao; backend pode ignorar contextual_evidence sem quebrar snapshots legados.

## Metricas

- minimized_collectors: 6 coletores listados como minimizados.
- sensitive_mode: true.
- raw_secret_logging: false.

## Resultado

- Status: pass
- Evidencia bruta anexada: internal/bootstrap/runtime_snapshot_test.go TestBuildContextualEvidenceMinimalProfileAvoidsRawSecrets.
- Observacoes: valida minimizacao e redaction local; nao substitui auditoria contratual especifica de cliente regulado.
- Rollback validado: variaveis de privacidade sao opt-in e compatibilidade do snapshot e aditiva.
- Revisor: Codex
- Aprovacao fechamento: yes
