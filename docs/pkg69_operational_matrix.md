# PKG-69 - Matriz operacional real

## Objetivo

Fechar a homologacao operacional dos pacotes PKG-59 a PKG-68 com evidencia por ambiente, falha e capacidade. Build/check local nao basta para declarar maturidade.

## Comando reproduzivel

```bash
scripts/pkg69_operational_homologation.sh
```

O script de homologacao executa:

- deteccao de Docker, kubectl, Helm e PowerShell;
- testes focados dos coletores e contratos adicionados em PKG-59 a PKG-68;
- cenarios locais automatizados de API indisponivel, backoff, payload grande e outbox cheia;
- `./check.sh`;
- listagem explicita de cenarios pendentes de ambiente real.

Smokes por sistema operacional:

```bash
scripts/smoke.sh
powershell -ExecutionPolicy Bypass -File scripts/smoke.ps1
```

Os smokes geram `smoke-evidence.json` no diretório temporário ou no caminho definido por
`SMOKE_EVIDENCE_FILE`/`-SmokeEvidenceFile`, com health, métricas, ingestão de logs e SHA256
dos artefatos locais usados na validação.

## Ambientes obrigatorios

| Ambiente | Evidencia exigida | Estado atual |
| --- | --- | --- |
| Windows Server | `smoke.ps1`, EventLog, servico Windows, update/rollback | pendente real |
| Windows desktop | `smoke.ps1`, EventLog, instalador, proxy | pendente real |
| Ubuntu/Debian | `smoke.sh`, systemd, logs, outbox, update | pendente real |
| RHEL/Alma/Rocky | `smoke.sh`, systemd, dnf/yum, update | pendente real |
| Docker | `CONTAINER_ENABLED=true`, Docker socket, labels, recursos | pendente |
| Kubernetes | DaemonSet/Helm, RBAC minimo, pods/events, rollback chart | pendente |
| macOS/dev local | testes focados e `./check.sh` | validavel por script |

## Cenarios obrigatorios

| Cenario | Evidencia exigida | Estado atual |
| --- | --- | --- |
| API indisponivel | backoff, outbox preservada, sem crash | parcial local |
| Rede intermitente | retry/backoff e flush posterior | parcial local |
| Proxy/TLS | proxy autenticado e TLS invalido controlado | pendente |
| Disco cheio/outbox cheia | erro claro, sem corrupcao | parcial local |
| Payload grande | limite/queda controlada em DogStatsD, OTLP e logs | parcial local |
| Clock errado | health/time sync com diagnostico | pendente |
| Reboot durante coleta | servico retorna e outbox preserva dados | pendente |
| Update quebrado | `update-report` com falha e rollback seguro | pendente |
| Permissao insuficiente | degradacao com status claro | pendente |
| Kubernetes RBAC minimo | sem permissao a secrets/exec/delete | pendente |
| Alto volume simultaneo | CPU/memoria dentro do limite definido | pendente |

## Limites aceitaveis iniciais

| Perfil | CPU media | Memoria RSS | Observacao |
| --- | ---: | ---: | --- |
| idle | <= 2% | <= 150 MB | host sem coletores avancados |
| coleta normal | <= 5% | <= 250 MB | sysmetrics, logs e health |
| alto volume local | <= 15% | <= 500 MB | DogStatsD/OTLP/logs simultaneos |
| containers/Kubernetes | <= 10% | <= 400 MB | intervalos >= 30s |

Limites devem ser revisados com evidencia real antes de declarar superioridade.

## Fechamento

PKG-69 so pode ser fechado quando:

- cada ambiente aplicavel tiver evidencia anexada ou bloqueio real documentado;
- cenarios marcados como `parcial local` forem reexecutados em ambiente real quando dependerem de SO/rede/disco/carga;
- `./check.sh` passar no `aiceberg_agent` e no `aiceberg_web`;
- artefatos oficiais forem gerados com SHA256;
- download/update remoto controlado for testado;
- matriz web `docs/agente_datadog_paridade.md` refletir validado vs pendente.
