# PKG-32 Lote 14 - Homologacao ponta a ponta

Data: 2026-04-26

Ambiente: lab local Docker/Colima em `/Users/brenomayder/projects/simulacao/aiceberg_lab`.

## Comandos executados

- `scripts/lab-simulacao.sh doctor`
- `colima start`
- `scripts/lab-simulacao.sh up`
- `scripts/lab-simulacao.sh validate`
- `scripts/lab-simulacao.sh acceptance`
- `scripts/lab-simulacao.sh status`
- `scripts/lab-simulacao.sh agent-logs node-direct`
- `scripts/lab-simulacao.sh agent-logs node-hub`
- `scripts/lab-simulacao.sh agent-logs node-relay`

## Evidencias geradas

- Validacao tecnica: `/Users/brenomayder/projects/simulacao/aiceberg_lab/shared/evidence/20260426_192730/checklist.md`
- Aceite ponta a ponta: `/Users/brenomayder/projects/simulacao/aiceberg_lab/shared/evidence/acceptance_20260426_192753/acceptance_checklist.md`
- Resumo de aceite: `/Users/brenomayder/projects/simulacao/aiceberg_lab/shared/evidence/acceptance_20260426_192753/acceptance_summary.txt`
- Snapshots da segunda validacao de aceite: `/Users/brenomayder/projects/simulacao/aiceberg_lab/shared/evidence/acceptance_20260426_192753/validation_runs/20260426_192827/snapshots`

## Resultado

| Fluxo | Resultado | Evidencia |
| --- | --- | --- |
| Agente `direct` | Aprovado | Processo ativo em modo `direct`, coleta local, buffer e flush registrados nos logs do container. |
| `hub` | Aprovado | Processo ativo em modo `hub`, listener `:9090`, coleta agentless e flush de lote registrados. |
| `relay` atras do hub | Aprovado | Processo ativo em modo `relay`, entrega para hub aprovada e guardrail confirmou ausencia de internet direta. |
| Update remoto | Aprovado por contrato automatizado | `internal/bootstrap/self_update_test.go` cobre etapas, falha classificada, proxy via hub e `version_confirmed`; o lab confirmou launcher/apply disponiveis nos tres modos. |
| Coleta grande | Aprovado por contrato automatizado e lab | `internal/bootstrap/channel_collect_test.go` cobre chunks, progresso e resultado; o lab validou lote agentless via hub. |

## Ressalvas

- A validacao tecnica do lab terminou como `APROVADO_COM_RESSALVAS`: 38 PASS, 2 WARN e 0 FAIL.
- Os WARN sao fluxos laterais opcionais (`app->app` e retorno `service->app`) fora do criterio central do canal.
- Esta homologacao nao substitui o release gate do Lote 15: `./check.sh` verde, rollback documentado e geracao da nova versao do agente.
