# PKG-69 - Update Remoto e Rollback

## Ambiente

- Data UTC: 2026-06-19T04:07:22Z
- Responsavel: Codex
- Cliente/lab: lab Docker root isolado e httptest remoto assinado
- Ambiente: Update remoto controlado
- Host/agente/HUB/relay: container Debian controlado e backend httptest local
- Versao agente: 0.8.8
- Artefato instalado: pacote tgz assinado Ed25519 com SHA256 validado e pacote invalido para rollback controlado
- Topologia: direct -> AIceberg

## Evidencia obrigatoria

- artefato assinado baixado: yes
- SHA256 validado: yes
- update aplicado: apply_failed com rollback controlado
- falha induzida reverteu: yes
- version_confirmed reportado: yes

## Metricas

- version_before: 0.8.8
- version_after: 0.8.8
- rollback_version: 0.8.8
- update_report_status: apply_failed

## Resultado

- Status: pass
- Evidencia bruta anexada: raw/aiceberg_pkg69_remote_update_raw_20260619T040558Z.tgz
- Observacoes: download remoto assinado Ed25519 validado; update-report incluiu download_ok, version_confirmed e apply_failed; apply script em container root restaurou binario anterior apos falha induzida
- Rollback validado: yes
- Revisor: Codex
- Aprovacao fechamento: yes
