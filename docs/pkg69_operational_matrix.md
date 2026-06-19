# PKG-69 - Matriz operacional real

## Objetivo

Fechar a homologacao operacional dos pacotes PKG-59 a PKG-68 com evidencia por ambiente, falha e capacidade. Build/check local nao basta para declarar maturidade.

## Comando reproduzivel

```bash
scripts/pkg69_operational_homologation.sh
```

Gate de fechamento com evidencias reais:

```bash
PKG69_TEMPLATE_DIR=/tmp/aiceberg_pkg69_templates \
PKG69_EVIDENCE_MANIFEST_TSV=/tmp/aiceberg_pkg69_manifest.tsv \
scripts/pkg69_operational_evidence_gate.sh
```

Para empacotar uma evidencia real ja coletada:

```bash
scripts/pkg69_bundle_evidence.sh relay-hub-direct-hosts /tmp/template-preenchido.md /tmp/evidencia-bruta
```

Para coletar evidencia read-only no host real e gerar o bundle:

```bash
PKG69_RUN_SMOKE=true scripts/pkg69_collect_host_evidence.sh linux-debian /tmp/template-preenchido.md
```

Para rodar o gate a partir de bundles ja coletados:

```bash
PKG69_EVIDENCE_MANIFEST_TSV=/tmp/aiceberg_pkg69_manifest.tsv \
scripts/pkg69_run_evidence_gate_from_bundles.sh /tmp/aiceberg_pkg69_bundles
```

Para gerar relatorio de lacunas:

```bash
PKG69_GAP_REPORT_FILE=/tmp/aiceberg_pkg69_gap_report.md \
scripts/pkg69_evidence_gap_report.sh /tmp/aiceberg_pkg69_bundles
```

Para usar o relatorio como bloqueio de fechamento/CI:

```bash
PKG69_GAP_REPORT_REQUIRE_COMPLETE=true \
scripts/pkg69_evidence_gap_report.sh /tmp/aiceberg_pkg69_bundles
```

Para bloquear fechamento sem todos os anexos reais:

```bash
PKG69_REQUIRE_REAL_EVIDENCE=true scripts/pkg69_operational_evidence_gate.sh
PKG69_REQUIRE_CLOSURE_ACCEPTED=true scripts/pkg69_operational_evidence_gate.sh
```

O script de homologacao executa:

- deteccao de Docker, kubectl, Helm e PowerShell;
- testes focados dos coletores e contratos adicionados em PKG-59 a PKG-68;
- validacao local da cadeia Ed25519 de artefato de update, aceitando assinatura valida e rejeitando assinatura invalida antes do `apply`;
- validacao local de reconexao pos-update: `version_confirmed` quando a versao aplica e `apply_failed/version_mismatch_after_restart` quando rollback mantem a versao anterior;
- validacao local de download via `HTTP_PROXY` autenticado, rejeicao padrao de TLS invalido e timeout sem finalizar artefato parcial;
- validacao local de clock skew com `time_sync.status` em `ok`, `warning` e `critical`, preservando clamp de offset extremo;
- validacao local de degradacao PCAP/tcpdump com permissao insuficiente, interface invalida e captura vazia sem crash;
- validacao local de restart/reabertura da outbox bbolt preservando itens antes/depois do restart e ACK parcial;
- validacao local de burst de alta cardinalidade em `custom_metrics`, com limite de series e contagem de drops;
- validacao local de topologia `direct -> AIceberg`, `hub -> AIceberg` e `relay -> hub -> AIceberg` para canal, ping legado, self-heal, update via proxy do Hub e encaminhamento Hub;
- e2e local multi-processo com backend fake, agente Hub, agente Relay e agente Direct, validando ingest por token, ping legado, bootstrap e Agentless jobs/observations;
- smoke POSIX local com evidencia de RSS, CPU e goroutines em coleta normal;
- cenarios locais automatizados com testes focados e logs dedicados para API indisponivel (`/tmp/aiceberg_pkg69_api_unavailable_test.log`), backoff/rede intermitente (`/tmp/aiceberg_pkg69_network_backoff_test.log`), payload grande (`/tmp/aiceberg_pkg69_payload_large_test.log`) e outbox cheia (`/tmp/aiceberg_pkg69_outbox_full_test.log`);
- `./check.sh`;
- listagem explicita de cenarios pendentes de ambiente real.

O gate `scripts/pkg69_operational_evidence_gate.sh` gera templates por ambiente/cenario, valida titulo, `Data UTC` em formato `YYYY-MM-DDTHH:MM:SSZ`, status `pass`, campos obrigatorios, topologia preenchida sem placeholder, anexo local existente e nao vazio em `Evidencia bruta anexada`, rollback validado, aprovacao de fechamento e SHA256/tamanho do template e do anexo bruto no manifest TSV. O helper `scripts/pkg69_bundle_evidence.sh` prepara esse pacote de evidencia a partir de um template preenchido e um arquivo/diretorio bruto, atualizando o campo de anexo e criando `MANIFEST.tsv` e `PROVENANCE.tsv`; ele aceita apenas cenarios oficiais do gate e seu self-test roda no `./check.sh`. O coletor `scripts/pkg69_collect_host_evidence.sh` executa comandos read-only no host, redige variaveis sensiveis de ambiente, registra `COLLECTION_SUMMARY.tsv`, pode rodar `scripts/smoke.sh` com `PKG69_RUN_SMOKE=true` e entao chama o helper de bundle; seu self-test tambem roda no `./check.sh`. O runner `scripts/pkg69_run_evidence_gate_from_bundles.sh` varre bundles, exige `MANIFEST.tsv` com uma unica evidencia, `template` coerente com `evidence.md`, `created_at_utc` em formato `YYYYMMDDTHHMMSSZ` e artefato do manifest coerente com `Evidencia bruta anexada`, exige `PROVENANCE.tsv` com ferramenta/cenario/data/artefato coerentes, exige `COLLECTION_SUMMARY.tsv` e `COMMANDS.tsv` em bundles `raw-host`, valida o cenario do resumo contra o manifest, exige contadores numericos, coerentes e iguais aos comandos registrados, valida SHA256/tamanho do `evidence.md` e do anexo bruto, mapeia o `scenario` para a variavel `PKG69_*_EVIDENCE` correta e chama o gate oficial; seu self-test cobre bundle adulterado, manifest com linha extra, timestamp invalido, artefato divergente, template divergente, cenario desconhecido, bundles parciais e falha quando `PKG69_REQUIRE_REAL_EVIDENCE=true` sem todos os anexos. O relatorio `scripts/pkg69_evidence_gap_report.sh` transforma o manifest TSV em Markdown e stdout estruturado com veredito de fechamento, status, motivo, proxima acao por cenario, `closure_acceptance` e pode falhar em modo bloqueante com `PKG69_GAP_REPORT_REQUIRE_COMPLETE=true` enquanto houver pendencia ou com `PKG69_GAP_REPORT_REQUIRE_ACCEPTED=true` quando faltar `PKG69_ACCEPT_CLOSURE=true`; esse modo tambem ativa rejeicao de marcadores sinteticos no gate. O self-test cobre rejeicao de sintetico no aceite, `PRONTO_PARA_REVISAO` e `ACEITO_PARA_FECHAMENTO` com 14/14 cenarios de lab controlado para evitar regressao do veredito e do aceite. Campos criticos tambem sao validados por conteudo, incluindo metricas numericas, limites iniciais de CPU/RSS, `ingest_confirmed=yes|true`, `recovered=yes|true`, `version_confirmed reportado=yes|true`, `status_before/status_after` em `ok|warning|critical`, `update_report_status` em `success|rolled_back|apply_failed`, RBAC Kubernetes sem `secrets/exec/delete`, hosts distintos para Direct/Hub/Relay, `relay_upstream_host_id=hub_host_id`, campos textuais da topologia Relay/Hub/Direct e Agentless via Hub positivos, `relay_direct_api_attempts=0` para a topologia `relay -> hub -> AIceberg` e ausencia de marcadores self-test/sinteticos/fake/mock/placeholder quando o modo real ou aceite de fechamento estiver ativo. O self-test `scripts/pkg69_operational_evidence_gate_selftest.sh` roda dentro de `./check.sh`.

Execucao local em 2026-06-18, Darwin 25.5.0 arm64:

- `scripts/pkg69_operational_homologation.sh`: passou com `go test` focado, cadeia Ed25519 local de artefato de update, reconexao pos-update local com `version_confirmed` e `version_mismatch_after_restart`, `HTTP_PROXY` autenticado local, TLS invalido rejeitado por padrao, timeout de download sem artefato finalizado, classificacao local de clock skew, degradacao local PCAP/tcpdump, replay de outbox apos restart local, burst local de cardinalidade custom metrics, topologia relay -> hub -> AIceberg em canal/ping/self-heal/update, e2e local multi-processo direct/hub/relay, smoke POSIX com RSS/CPU/goroutines locais, testes focados de API indisponivel/rede intermitente/payload grande/outbox cheia com logs dedicados, gate endurecido para topologia preenchida, anexo bruto local nao vazio com hash/tamanho no manifest, helper de bundle, coletor read-only, runner de bundles e relatorio de lacunas com self-tests, rollback validado, limites CPU/RSS e RBAC Kubernetes e `./check.sh`;
- Docker daemon indisponivel no host local;
- `kubectl` disponivel, mas sem validacao de cluster/DaemonSet/Helm;
- Helm e PowerShell indisponiveis neste host;
- Windows, Linux real, Docker, Kubernetes, proxy real/TLS, disco cheio real, alto volume real e rollback de update seguem pendentes de ambiente controlado.

Smokes por sistema operacional:

```bash
scripts/smoke.sh
powershell -ExecutionPolicy Bypass -File scripts/smoke.ps1
```

Os smokes geram `smoke-evidence.json` no diretório temporário ou no caminho definido por
`SMOKE_EVIDENCE_FILE`/`-SmokeEvidenceFile`, com health, métricas, ingestão de logs e SHA256
dos artefatos locais usados na validação.
Em hosts reais sem toolchain Go, `scripts/smoke.sh` aceita `SMOKE_AGENT_BIN` e
`SMOKE_BACKEND_BIN` apontando para binários pré-compilados; sem essas variáveis, mantém o
comportamento padrão de compilar localmente.

Smoke POSIX local executado em 2026-06-18 com `SMOKE_EVIDENCE_FILE=/tmp/aiceberg_pkg69_smoke_evidence.json`:

- `health.status=ok`;
- `agent_pipeline_version=2-compatible`;
- ingestao confirmada em `/v1/ingest/bootstrap`, `/v1/ingest/health`, `/v1/ingest/metrics` e `/v1/logs/raw`;
- `agent_log_sha256=55fa32917610298090284f24906b144030424ccb64212da613aa0af7743beae5`;
- `oslog_fixture_sha256=e14a2773b6da308c2776891300d37c55cd9750137af86071d40b7655c0a35525`.

Artefatos oficiais 0.8.8 gerados em 2026-06-18:

- `./check.sh` passou antes da geracao;
- `./scripts/build_installers.sh` gerou Linux amd64/arm64, Darwin amd64/arm64 e Windows amd64;
- compactados publicados localmente em `aiceberg_web/cliente/web/downloads/agent/0.8.8/`;
- `shasum -a 256 -c SHA256SUMS` passou na pasta publicada;
- download/update remoto controlado ainda nao foi acionado.

Hashes 0.8.8:

| Arquivo | SHA-256 |
| --- | --- |
| `aiceberg-agent-darwin-amd64.tar.gz` | `4f93259351a1c7c80a57b5a94d62cc25f80af7d6dce3e21b5d3f7e7fd5f872d3` |
| `aiceberg-agent-darwin-arm64.tar.gz` | `b1ba2d6322e4a9b34208d565632fa14b6dbae99716f25f0949f87c5cec5be0a6` |
| `aiceberg-agent-linux-amd64.tar.gz` | `7ec788d694956850f8d8e9a1d864d46b764cef0b3473bd5020586cdf4858a9d8` |
| `aiceberg-agent-linux-arm64.tar.gz` | `c9846007c6a7ff5a62ae77b831e17ff4ed8598ffde948c2869d22e143956dabf` |
| `aiceberg-agent-windows-amd64.zip` | `482ede3f6a7d8758dd4c00b341d2cad92f4be23014a9a6209827dd3f644cceb5` |

Execucao real controlada em 2026-06-19, `VMAIPROD2`, Oracle Linux/RHEL-like 6.12.0, cliente `S&S Soluções em TI LTDA` (`nome_fantasia=InspectApp`), agente id `4`:

- instalacao cirurgica do binario Linux amd64 `0.8.8` em `/usr/local/bin/aiceberg_agent`, preservando a unit legada e sem reiniciar nginx/php-fpm/filas do AIceberg;
- SHA-256 instalado: `7ec788d694956850f8d8e9a1d864d46b764cef0b3473bd5020586cdf4858a9d8`;
- backup remoto criado em `/var/www/aiceberg/runtime/deploy_backups/20260619T004918Z_pkg69_agent_088_loopback`;
- correcao operacional do agente para `API_BASE_URL=http://api.aiceberg.com.br` com resolucao local `api.aiceberg.com.br -> 127.0.0.1`, evitando timeout hairpin do dominio publico a partir da propria VM;
- `AGENT_CLIENT_ID=1`, `AGENT_ID=4`, `AGENT_MODE=direct` e `AGENTLESS_ENABLED=false`, com backup adicional em `/var/www/aiceberg/runtime/deploy_backups/20260619T005028Z_pkg69_disable_agentless_direct`;
- `systemctl is-active aiceberg-agent.service`: `active`; `NRestarts=0`;
- logs do agente: `bootstrap ok`, `channel opened mode=direct`, coletas `bootstrap`, `inventory`, `health` e `metrics`, `flushed ... acked ... retained=0`, `config sync ok version=1`;
- banco remoto: `agente_snapshot` recebeu registros recentes para `agente_id=4`, incluindo `bootstrap` em `2026-06-19 00:50:46` e `metrics` em `2026-06-19 00:51:45`;
- tela `cliente.aiceberg.com.br/agente/view?id=4`: `Ping: 71.0 ms`, `Online real`, `Canal ativo`, `Versão / OS 0.8.8 · linux / amd64`, `Topologia agent -> AIceberg`, IP remoto `127.0.0.1`.

Smoke remoto oficial em 2026-06-19 no mesmo host, isolado em `/tmp/aiceberg_pkg69_official_smoke_20260619T010125Z`, com backend fake local e binários Linux amd64 pré-compilados:

- `scripts/smoke.sh` executado com `SMOKE_AGENT_BIN`, `SMOKE_BACKEND_BIN`, `SMOKE_WORKDIR` e `SMOKE_KEEP=1`;
- `smoke-evidence.json` SHA-256 `5fb11b6728dac1328e31ba2829090c0c95e55313f214fdc01ec9cf9a347e0a10`;
- binário agente SHA-256 `fd32a7d00e18eaeb4f26b55a39676783d5a9959c8e20c67d1a6b6c5234787341`;
- backend fake SHA-256 `74e0b1df02d88ee1c50eb8e02d56e0db0e0866cef60ef82ea928e0738a3eaef7`;
- `health.status=ok`, `agent_pipeline_version=2-compatible`, `proc_rss_bytes=13516800`, `proc_cpu_percent=3.6125293173687845`, `goroutines=8`;
- ingestao confirmada em `/v1/ingest/bootstrap`, `/v1/ingest/health`, `/v1/ingest/metrics` e `/v1/logs/raw`;
- apos o smoke, o servico real `aiceberg-agent.service` permaneceu `active`, `MainPID=2529804`, `NRestarts=0`, com coletas recentes no journal.

Esta evidencia real ainda nao fecha o cenario `linux-rhel` no gate porque rollback real, update remoto assinado e os demais cenarios obrigatorios nao foram executados. Ela valida instalacao Linux/RHEL-like, smoke POSIX oficial, bootstrap, canal, ingestao e flush no servidor real do AIceberg sem derrubar a aplicacao.

## Ambientes obrigatorios

| Ambiente | Evidencia exigida | Estado atual |
| --- | --- | --- |
| Windows Server | `smoke.ps1`, EventLog, servico Windows, update/rollback | pendente real |
| Windows desktop | `smoke.ps1`, EventLog, instalador, proxy | pendente real |
| Ubuntu/Debian | `smoke.sh`, systemd, logs, outbox, update | pendente real |
| RHEL/Alma/Rocky | `smoke.sh`, systemd, dnf/yum, update | parcial real em `VMAIPROD2`: instalacao 0.8.8, systemd ativo, bootstrap/canal/flush/snapshot OK e `scripts/smoke.sh` remoto OK; rollback/update real pendentes |
| Docker | `CONTAINER_ENABLED=true`, Docker socket, labels, recursos | pendente |
| Kubernetes | DaemonSet/Helm, RBAC minimo, pods/events, rollback chart | pendente |
| macOS/dev local | testes focados, `./check.sh` e smoke POSIX | validado local em 2026-06-18 |

## Cenarios obrigatorios

| Cenario | Evidencia exigida | Estado atual |
| --- | --- | --- |
| API indisponivel | backoff, outbox preservada, sem crash | parcial local |
| Rede intermitente | retry/backoff e flush posterior | parcial local |
| Proxy/TLS | proxy autenticado e TLS invalido controlado | parcial local para `HTTP_PROXY` e rejeicao TLS invalido; proxy real/TLS pendente |
| Disco cheio/outbox cheia | erro claro, sem corrupcao | parcial local |
| Payload grande | limite/queda controlada em DogStatsD, OTLP e logs | parcial local |
| Clock errado | health/time sync com diagnostico | parcial local para `time_sync.status`; NTP/clock real pendente |
| Reboot durante coleta | servico retorna e outbox preserva dados | parcial local para reabertura bbolt; reboot real pendente |
| Update quebrado | `update-report` com falha e rollback seguro | parcial local; cadeia Ed25519, timeout de download, `version_confirmed` e mismatch apos rollback cobertos localmente; update remoto real pendente |
| Relay/Hub/Direct | relay envia somente para Hub; Hub encaminha ao AIceberg | parcial local com e2e multi-processo; smoke real em hosts separados pendente |
| Permissao insuficiente | degradacao com status claro | parcial local para PCAP/tcpdump; host real pendente |
| Kubernetes RBAC minimo | sem permissao a secrets/exec/delete | pendente |
| Alto volume simultaneo | CPU/memoria dentro do limite definido | parcial local para cardinalidade custom metrics e overhead de coleta normal; carga real CPU/mem pendente |

## Limites aceitaveis iniciais

| Perfil | CPU media | Memoria RSS | Observacao |
| --- | ---: | ---: | --- |
| idle | <= 2% | <= 150 MB | host sem coletores avancados |
| coleta normal | <= 5% | <= 250 MB | sysmetrics, logs e health; smoke POSIX local registra RSS/CPU/goroutines |
| alto volume local | <= 15% | <= 500 MB | DogStatsD/OTLP/logs simultaneos |
| containers/Kubernetes | <= 10% | <= 400 MB | intervalos >= 30s |

Limites devem ser revisados com evidencia real antes de declarar superioridade.

## Fechamento

PKG-69 so pode ser fechado quando:

- cada ambiente aplicavel tiver evidencia anexada ou bloqueio real documentado;
- cenarios marcados como `parcial local` forem reexecutados em ambiente real quando dependerem de SO/rede/disco/carga;
- `./check.sh` passar no `aiceberg_agent` e no `aiceberg_web`;
- artefatos oficiais forem gerados com SHA256;
- download/update remoto controlado for testado com artefato assinado;
- matriz web `docs/agente_datadog_paridade.md` refletir validado vs pendente.
- `scripts/pkg69_operational_evidence_gate.sh` tiver todos os anexos reais com manifest pronto e fechamento explicitamente aceito.
