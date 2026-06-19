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

Validacao real de API indisponivel em 2026-06-19 no `VMAIPROD2`, isolada em `/tmp/aiceberg_pkg69_api_down_20260619T010823Z`, com binario temporario e porta local fechada:

- antes da correcao, o agente encerrava no bootstrap com `[FATAL] bootstrap failed` quando a API nao respondia;
- `runInitialBootstrap` agora registra `bootstrap degraded`, permite que health, canal e schedulers subam e agenda retry controlado do bootstrap inicial;
- `api-down-evidence.json` SHA-256 `043d754cdb2fc4e9e04edd53cfd539b8bfc24f8d5a22d79f37301cea5b3ae828`;
- binario temporario SHA-256 `3b286a3903fcba71b182c0e178bb309ff5720c57facd88f4677facdd2e794a2b`;
- checks reais: `process_alive=true`, `health_endpoint=true`, `bootstrap_degraded_logged=true`, `fatal_absent=true`, `channel_fallback_active=true`;
- health real: `status=ok`, `agent_pipeline_version=2-compatible`, `proc_rss_bytes=13623296`, `proc_cpu_percent=2.5912926792576854`, `goroutines=6`;
- o servico instalado `aiceberg-agent.service` permaneceu `active`, `MainPID=2529804`, `NRestarts=0`.

Validacao real de recuperacao apos retorno da API em 2026-06-19 no `VMAIPROD2`, isolada em `/tmp/aiceberg_pkg69_api_recovery_20260619T011438Z`, com backend fake iniciado depois do agente:

- `api-recovery-evidence.json` SHA-256 `1458611ad22e5272b494c76a283ffcd6b10bef7156fdfde98464bad62eb0dd4b`;
- binario temporario SHA-256 `8dab1de2968cdf6094ab62efa289c1e35b0c794177050206f78f286fd1b88bb3`;
- `bootstrap_degraded_logged=true`, `bootstrap_retry_ok=true`, `backend_bootstrap=true`, `ping_recovered=true`, `ingest_recovered=true`, `health_endpoint=true`;
- backend fake recebeu `bootstraps=1`, `config_gets=2`, `ping_get=2`, `ping_post=2`, ingestao em `/v1/ingest/bootstrap=1` e `/v1/ingest/health=1`;
- health real: `status=ok`, `agent_pipeline_version=2-compatible`, `flush_ok=2`, `proc_rss_bytes=16588800`, `proc_cpu_percent=4.83237575441734`, `goroutines=14`;
- o servico instalado `aiceberg-agent.service` permaneceu `active`, `MainPID=2529804`, `NRestarts=0`.

Validacao real parcial nos servidores Inspect autorizados em 2026-06-19:

- `old-plesk-92.204.168.1`, agente esperado `19`, Debian 11, `aiceberg-agent.service` ja instalado e ativo: smoke temporario isolado em `/tmp/aiceberg_pkg69_old_plesk_19_smoke_20260619T012221Z`, com backend fake local e binario Linux amd64 temporario SHA-256 `a3e03f930857f1bfc3c8a8e033f9adf53bb8e5e727f1072bc9720ec1fe451023`;
- evidencia `evidence.json` SHA-256 `91992345bba689fc4e1e5bc20f769bb58873e398750f7504838c92482b5f39f8`;
- checks: `health_endpoint=true`, `metrics_endpoint=true`, `logs_ingested=true`, `installed_service_still_active=true`;
- backend fake recebeu `bootstraps=1`, `config_gets=3`, `ping_get=8`, `ping_post=8`, ingestao em `/v1/ingest/bootstrap`, `/v1/ingest/health`, `/v1/ingest/metrics` e `/v1/logs/raw`;
- health temporario: `status=ok`, `agent_pipeline_version=2-compatible`, `proc_rss_bytes=15962112`, `proc_cpu_percent=4.877856900533356`, `goroutines=15`;
- o servico instalado permaneceu `active/running`, `MainPID=913797`, `NRestarts=0`; o host esta com disco em 97%, entao disco cheio real permanece pendente de ambiente controlado e nao deve ser forcado nesse servidor;
- `old-plesk-92.42.106.121`, agente esperado `1`, bloqueou acesso nao interativo (`Permission denied publickey,password`); fica pendente ate disponibilizar chave ou procedimento nao interativo seguro;
- servidor novo `187.45.180.181`, Ubuntu 24.04, sem unit AIceberg instalada (`LoadState=not-found`): smoke temporario isolado em `/tmp/aiceberg_pkg69_new_inspect_smoke_20260619T012423Z`, com backend fake local e mesmo binario temporario;
- evidencia `evidence.json` SHA-256 `b325aeea690f72c853d595239c85451c1925e34b72334fe28e6955d82ee5f20b`;
- checks: `health_endpoint=true`, `metrics_endpoint=true`, `logs_ingested=true`; backend fake recebeu `bootstraps=1`, `config_gets=3`, `ping_get=6`, `ping_post=6`, ingestao em `/v1/ingest/bootstrap`, `/v1/ingest/health`, `/v1/ingest/metrics` e `/v1/logs/raw`;
- health temporario: `status=ok`, `agent_pipeline_version=2-compatible`, `proc_rss_bytes=17932288`, `proc_cpu_percent=12.162803612228979`, `goroutines=15`; como CPU ficou acima do limite inicial de coleta normal e nao houve instalacao systemd/update/rollback, esta evidencia conta apenas como smoke POSIX parcial, nao como fechamento do ambiente Ubuntu/Debian.

Atualizacao/instalacao real adicional em 2026-06-19:

- `old-plesk-92.42.106.121`, agente `1`: acesso por senha, backup em `/root/aiceberg_pkg69_agent1_backup_20260619T021006Z`, binario atualizado, `AGENT_CLIENT_ID=1`, `AGENT_ID=1`, `AGENT_MODE=direct`, `API_BASE_URL=https://api.aiceberg.com.br`, restart controlado, `MainPID=1864398`, `NRestarts=0`, canal direct abriu e flush ACK apareceu no journal; AIceberg registrou `seen=2026-06-19 01:49:26`, `version=0.8.8`, `os=linux`;
- `92.204.189.13`, agente `18`: acesso por senha, backup em `/root/aiceberg_pkg69_agent18_backup_20260619T015429Z`, binario atualizado SHA-256 `a3e03f930857f1bfc3c8a8e033f9adf53bb8e5e727f1072bc9720ec1fe451023`, `AGENT_CLIENT_ID=1`, `AGENT_ID=18`, `AGENT_MODE=direct`, `API_BASE_URL=https://api.aiceberg.com.br`, restart controlado, `MainPID=455470`, `NRestarts=0`, canal direct abriu, flush ACK e `config sync ok` apareceram no journal; AIceberg registrou `seen=2026-06-19 01:55:32`; como o bootstrap local foi pulado por `state_found`, o campo operacional `versao_agente` foi sincronizado para `0.8.8` apos confirmar hash/binario;
- `old-plesk-92.204.168.1`, agente `19`: backup em `/root/aiceberg_pkg69_agent19_backup_20260619T013919Z`, binario atualizado, `AGENT_CLIENT_ID=1`, `AGENT_ID=19`, `AGENT_MODE=direct`, `API_BASE_URL=https://api.aiceberg.com.br`, `MainPID=78154`, `NRestarts=0`, health local `status=ok`, `agent_pipeline_version=2-compatible`, `proc_rss_bytes=16945152`, `proc_cpu_percent=6.345314950432266`; AIceberg registrou `seen=2026-06-19 01:49:33`, `version=0.8.8`, `os=linux`;
- `187.45.180.181` Linux, agente `70`: cadastro criado no AIceberg para o cliente `1`, modo direct; como o usuario `deploy` nao tem `sudo -n`, a instalacao foi feita em modo usuario em `/home/deploy/aiceberg-agent`, com `@reboot /home/deploy/aiceberg-agent/start.sh` no crontab, `HEALTH_PORT=18081`, binario SHA-256 `a3e03f930857f1bfc3c8a8e033f9adf53bb8e5e727f1072bc9720ec1fe451023`, processo `pid=2085408`, health local `status=ok`, `agent_pipeline_version=2-compatible`, `proc_rss_bytes=18026496`, `proc_cpu_percent=7.163809801110169`, `goroutines=11`; AIceberg registrou `installed=2026-06-19 01:59:59`, `seen=2026-06-19 02:00:15`, `version=0.8.8`, `os=linux`; ainda nao substitui validacao systemd/root porque nao ha permissao sudo/root nesse acesso;
- coleta read-only adicional do agente `70` em `/tmp/aiceberg_pkg69_linux70_readonly_20260619T020840Z`: SHA-256 do artefato `28e3bfd6cae62bd67750172971ab19933d5d48dc99d04f01f519377ccb6e8110`, `status=ok`, `version=0.8.8`, `queue_items=1`, `flush_ok=68`, `proc_rss_bytes=20967424`, `proc_cpu_percent=7.837810919400362`, canal direct conectado, `relay_connects_to_aiceberg=false`, crontab presente, token/env presentes e `sudo_noninteractive=no`; nova consulta apos um ciclo manteve `status=ok`, `flush_ok=74`, `queue_items=1` e `proc_cpu_percent=7.772703271488744`, portanto continua evidência parcial e nao passa no limite inicial de CPU do gate;
- ajuste controlado do agente `70` com binario SHA-256 `54ee6fe293d30222845c9780b63d0bbb80331c0477ef73399f1f8d468c8ac397`, backup `/home/deploy/aiceberg-agent/bin/aiceberg_agent.backup_pkg69_metrics_interval_20260619T021650Z` e `METRICS_INTERVAL=60`: apos 88s, `ps` reportou CPU `1.7%`, health local reportou `proc_cpu_percent=1.7297253193545055`, `proc_rss_bytes=18673664`, `flush_ok=10`, canal direct conectado e `relay_connects_to_aiceberg=false`; ainda nao fecha Ubuntu/Debian porque systemd/root, update remoto assinado e rollback real do gate seguem pendentes;
- `10.100.35.3` Windows Server 2022, agente `71`: cadastro criado no AIceberg para o cliente `1`, smoke Windows real em `D:\aiceberg_pkg69_windows_smoke_20260619T013720Z\work`, evidencia SHA-256 `2f49f39ba8201bdf84d9e59520a405296401105e48e7be63a3203f15e0d84bfe`, `health_endpoint=true`, `metrics_endpoint=true`, `windows_eventlog_mode=true`, `debug_log_created=true`; instalacao como servico `AIcebergAgent` em `D:\AIcebergAgentInstall`, `service_before_exists=false`, `service_status=Running`, `startup=Automatic`, `service_pid=3564`, `HEALTH_PORT=8081`, health local `status=ok`, `agent_pipeline_version=2-compatible`, `proc_rss_bytes=29876224`, `proc_cpu_percent=5.92235371318283`, `goroutines=15`; AIceberg registrou `installed=2026-06-19 01:41:22`, `seen=2026-06-19 01:49:34`, `version=0.8.8`, `os=windows`;
- ajuste controlado do agente Windows `71` com binario SHA-256 `950d73174467ec3bbca4fac9ba255c9a9bc458d68fbee165fbedd370ae3b4c4e`, backup `C:\Program Files\AIceberg\agent\agent.exe.backup_pkg69_metrics_interval_20260619T022451Z` e `METRICS_INTERVAL=60` em ambiente de maquina: servico `AIcebergAgent` permaneceu `Running`, apos 94s health reportou `status=ok`, `proc_cpu_percent=1.3342751749640507`, `proc_rss_bytes=31277056`, `flush_ok=12`, canal direct conectado e `relay_connects_to_aiceberg=false`;
- rollback controlado do Windows Server `71` validado em 2026-06-19: troca para `agent.exe.backup_pkg69_metrics_interval_20260619T022451Z`, health OK com `proc_cpu_percent=3.425638705830247`, restore do binario atual SHA-256 `950d73174467ec3bbca4fac9ba255c9a9bc458d68fbee165fbedd370ae3b4c4e`, health final OK com `proc_cpu_percent=1.2083213528861931`, `proc_rss_bytes=31109120`, `flush_ok=12`, canal direct conectado e `relay_connects_to_aiceberg=false`; bundle versionado em `docs/evidence/pkg69/windows-server-20260619T023855Z`, e `scripts/pkg69_evidence_gap_report.sh docs/evidence/pkg69` reporta `1/14` evidencias OK;
- rollback controlado no `VMAIPROD2` Oracle Linux/RHEL-like validado em 2026-06-19: systemd `aiceberg-agent.service` permaneceu `active`, `NRestarts=0`, rollback para binario SHA-256 `0b9e6a71a564fb08a8e85865200648ee7383c5e1e9561673ae92dfc0583b4e3f`, restore do binario atual SHA-256 `fd32a7d00e18eaeb4f26b55a39676783d5a9959c8e20c67d1a6b6c5234787341`, journal com flush ACK e `retained=0`, CPU final `3.4%`, RSS `16748544`; bundle versionado em `docs/evidence/pkg69/linux-rhel-20260619T025113Z`, e `scripts/pkg69_evidence_gap_report.sh docs/evidence/pkg69` reporta `2/14` evidencias OK;
- rollback controlado no Debian 11 do agente `19` validado em 2026-06-19: systemd `aiceberg-agent.service` permaneceu `active`, `NRestarts=0`, rollback para binario SHA-256 `794426c0ecddd17e8ef5f50798504fee455b75e59ebcf0a2a5e2b42a66ba25fe`, restore do binario atual SHA-256 `a3e03f930857f1bfc3c8a8e033f9adf53bb8e5e727f1072bc9720ec1fe451023`, journal com flush ACK e `retained=0`, CPU final `3.0%`, RSS `20197376`; bundle versionado em `docs/evidence/pkg69/linux-debian-20260619T030202Z`;
- Docker Runtime validado em 2026-06-19 com daemon Docker real local em Linux VM do Docker Desktop: `CONTAINER_ENABLED=true`, socket `/var/run/docker.sock`, `/var/lib/docker/containers` read-only, container controlado `aiceberg-pkg69-log-probe-clean`, filtro de include para evitar logs de outros workloads, `containers_seen=1`, `container_logs_seen=7`, cursor JSON, CPU `0.0%`, RSS `9261056`, cleanup do container validado e bundle versionado em `docs/evidence/pkg69/docker-runtime-20260619T031843Z`; `scripts/pkg69_evidence_gap_report.sh docs/evidence/pkg69` reporta `4/14` evidencias OK;
- permissao eBPF/PCAP restrita validada em 2026-06-19 no Linux `187.45.180.181` com usuario `deploy`: `tcpdump -i any` retornou `Operation not permitted`, o coletor de rede reportou warning `pcap indisponivel: permissao insuficiente para tcpdump`, manteve `socket_snapshot+pcap` sem crash, o agente `70` seguiu `status=ok`, `flush_ok=566`, canal direct conectado, CPU `1.4%`; bundle versionado em `docs/evidence/pkg69/permission-ebpf-20260619T032753Z`; `scripts/pkg69_evidence_gap_report.sh docs/evidence/pkg69` reporta `5/14` evidencias OK;
- alto volume e overhead validados em 2026-06-19 com receptores reais locais dos coletores: DogStatsD 1500 series com limite de 1000, OTLP metrics/logs/traces 180 itens por tipo com limite de 100, `accepted_count=1300`, `dropped_count=740`, CPU `0.0%`, RSS `14567688`; bundle versionado em `docs/evidence/pkg69/high-volume-overhead-20260619T033436Z`; `scripts/pkg69_evidence_gap_report.sh docs/evidence/pkg69` reporta `6/14` evidencias OK;
- Proxy/TLS validado em 2026-06-19 com proxy HTTP autenticado local, TLS invalido rejeitado e TLS valido publico aceito pelo client HTTP do agente: `requests_ok=2`, `requests_failed_expected=1`, `retry_count=0`; bundle versionado em `docs/evidence/pkg69/proxy-tls-20260619T033945Z`; `scripts/pkg69_evidence_gap_report.sh docs/evidence/pkg69` reporta `7/14` evidencias OK;
- disco cheio validado em 2026-06-19 em volume APFS temporario de 16MB montado via `hdiutil`: outbox bbolt real recebeu item inicial, filesystem retornou `no space left on device`, push durante ENOSPC falhou sem alterar a fila, volume foi liberado/remontado logicamente e a outbox reabriu com replay preservado; `free_bytes_before=16359424`, `queue_items=2`, `recovered=yes`; bundle versionado em `docs/evidence/pkg69/disk-full-20260619T034504Z`; `scripts/pkg69_evidence_gap_report.sh docs/evidence/pkg69` reporta `8/14` evidencias OK;
- `scripts/smoke.ps1` foi ajustado para aceitar binarios pre-compilados, limpar processos filhos em `finally`, silenciar progresso do PowerShell e tratar Windows EventLog como modo proprio, sem exigir o fixture POSIX de `/v1/logs/raw`.

## Ambientes obrigatorios

| Ambiente | Evidencia exigida | Estado atual |
| --- | --- | --- |
| Windows Server | `smoke.ps1`, EventLog, servico Windows, update/rollback | OK no gate: bundle `docs/evidence/pkg69/windows-server-20260619T023855Z`, smoke/servico/health/EventLog/rollback controlado validados; update remoto assinado ainda fica no cenario `remote-update-rollback` |
| Windows desktop | `smoke.ps1`, EventLog, instalador, proxy | pendente real |
| Ubuntu/Debian | `smoke.sh`, systemd, logs, outbox, update | OK no gate para Debian: bundle `docs/evidence/pkg69/linux-debian-20260619T030202Z`, systemd/rollback/restore/journal/overhead validados no agente `19`; Ubuntu 24.04 id `70` ainda parcial por modo usuario/crontab; update remoto assinado fica no cenario `remote-update-rollback` |
| RHEL/Alma/Rocky | `smoke.sh`, systemd, dnf/yum, update | OK no gate: bundle `docs/evidence/pkg69/linux-rhel-20260619T025113Z`, systemd/rollback/restore/journal/overhead validados no `VMAIPROD2`; update remoto assinado ainda fica no cenario `remote-update-rollback` |
| Docker | `CONTAINER_ENABLED=true`, Docker socket, labels, recursos | OK no gate: bundle `docs/evidence/pkg69/docker-runtime-20260619T031843Z`, socket real, logs JSON com cursor, filtro de container controlado e cleanup validados |
| Kubernetes | DaemonSet/Helm, RBAC minimo, pods/events, rollback chart | pendente |
| macOS/dev local | testes focados, `./check.sh` e smoke POSIX | validado local em 2026-06-18 |

## Cenarios obrigatorios

| Cenario | Evidencia exigida | Estado atual |
| --- | --- | --- |
| API indisponivel | backoff, outbox preservada, sem crash | parcial real em `VMAIPROD2`: bootstrap degradado sem fatal, health OK, canal em fallback, retry de bootstrap e recuperacao de ping/config/ingest quando API volta; variacoes rede/proxy/disco ainda pendentes |
| Rede intermitente | retry/backoff e flush posterior | parcial local |
| Proxy/TLS | proxy autenticado e TLS invalido controlado | OK no gate: bundle `docs/evidence/pkg69/proxy-tls-20260619T033945Z`, proxy autenticado, TLS invalido rejeitado e TLS valido aceito |
| Disco cheio/outbox cheia | erro claro, sem corrupcao | OK no gate: bundle `docs/evidence/pkg69/disk-full-20260619T034504Z`, volume APFS temporario cheio, outbox preservada e recuperacao validada |
| Payload grande | limite/queda controlada em DogStatsD, OTLP e logs | parcial local |
| Clock errado | health/time sync com diagnostico | parcial local para `time_sync.status`; NTP/clock real pendente |
| Reboot durante coleta | servico retorna e outbox preserva dados | parcial local para reabertura bbolt; reboot real pendente |
| Update quebrado | `update-report` com falha e rollback seguro | parcial local; cadeia Ed25519, timeout de download, `version_confirmed` e mismatch apos rollback cobertos localmente; update remoto real pendente |
| Relay/Hub/Direct | relay envia somente para Hub; Hub encaminha ao AIceberg | parcial local com e2e multi-processo; smoke real em hosts separados pendente |
| Permissao insuficiente | degradacao com status claro | OK no gate: bundle `docs/evidence/pkg69/permission-ebpf-20260619T032753Z`, tcpdump sem permissao em Linux real, degradacao clara e ingest ativo validados |
| Kubernetes RBAC minimo | sem permissao a secrets/exec/delete | pendente |
| Alto volume simultaneo | CPU/memoria dentro do limite definido | OK no gate: bundle `docs/evidence/pkg69/high-volume-overhead-20260619T033436Z`, DogStatsD/OTLP metrics/logs/traces com accepted/dropped e overhead dentro do limite |

## Limites aceitaveis iniciais

| Perfil | CPU media | Memoria RSS | Observacao |
| --- | ---: | ---: | --- |
| idle | <= 2% | <= 150 MB | host sem coletores avancados |
| coleta normal | <= 5% | <= 250 MB | sysmetrics, logs e health; smoke POSIX local registra RSS/CPU/goroutines |
| alto volume local | <= 15% | <= 500 MB | DogStatsD/OTLP/logs simultaneos |
| containers/Kubernetes | <= 10% | <= 400 MB | intervalos >= 30s |

Limites devem ser revisados com evidencia real antes de declarar superioridade. A partir deste pacote, `METRICS_INTERVAL` permite aumentar o intervalo de sysmetrics sem recompilar; o default permanece `10` segundos para compatibilidade retroativa.

## Fechamento

PKG-69 so pode ser fechado quando:

- cada ambiente aplicavel tiver evidencia anexada ou bloqueio real documentado;
- cenarios marcados como `parcial local` forem reexecutados em ambiente real quando dependerem de SO/rede/disco/carga;
- `./check.sh` passar no `aiceberg_agent` e no `aiceberg_web`;
- artefatos oficiais forem gerados com SHA256;
- download/update remoto controlado for testado com artefato assinado;
- matriz web `docs/agente_datadog_paridade.md` refletir validado vs pendente.
- `scripts/pkg69_operational_evidence_gate.sh` tiver todos os anexos reais com manifest pronto e fechamento explicitamente aceito.
