# PKG-60 - Pipeline seguro de logs

## Escopo entregue

O coletor `oslogs` continua usando o contrato legado `/v1/logs/raw` e adiciona campos opcionais por evento:

- `schema_version`;
- `timestamp_utc`;
- `host`;
- `path` ou `channel`;
- `cursor`;
- `service`;
- `attributes`;
- `redaction_status`;
- `transport`;
- `source_tool`;
- `source_category`.

O payload tambem pode enviar `dropped_count` para indicar quantos eventos foram descartados por filtro local sem persistir conteudo descartado.

## Processamento

- Unix-like: leitura por arquivo com cursor persistente, reset seguro quando o arquivo e truncado ou rotacionado por troca de inode, agrupamento multiline simples e parser syslog existente.
- Windows: leitura por canais EventLog via `wevtutil`, cursor por `Record ID`, metadados de canal/provider/evento, filtros opcionais por provider/event_id e redaction antes do envio.
- Local POSIX: listeners opcionais TCP/UDP recebem linhas de aplicacoes locais, aplicam os mesmos limites, filtros e redaction, e entram no mesmo `/v1/logs/raw`.
- Journald POSIX: `journalctl --output=json` opcional e desligado por padrao, com filtros por unit e prioridade, cursor por `_SOURCE_REALTIME_TIMESTAMP`, argumentos sanitizados e redaction antes do envio.
- JSON: campos JSON de primeiro nivel sao copiados para `attributes`; chaves sensiveis sao mascaradas.
- Redaction: token, senha, secret, API key, cookie e `Authorization` sao mascarados antes de sair do host.
- Filtros: `OSLOG_INCLUDE_REGEX`, `OSLOG_EXCLUDE_REGEX` e `OSLOG_MIN_SEVERITY` podem ser definidos por env ou config remota em `logs.include_regex`, `logs.exclude_regex` e `logs.min_severity`.
- Processors opcionais: `parse`, `remap`, `drop`, `mask`, `route`, `sample` e `enrich` podem ser definidos por `OSLOG_PROCESSORS_JSON` ou `logs.processors`; quando vazios, o comportamento legado permanece.
- Dual-shipping controlado: `OSLOG_DUAL_SHIP_ENDPOINTS` ou `logs.dual_ship_endpoints` duplica o envelope ja sanitizado apenas para endpoints internos seguros `/v1/logs/*`; URLs externas e rotas fora de logs sao ignoradas.

## Compatibilidade

- `/v1/logs/raw` nao muda.
- `events[]` continua existindo.
- Campos novos sao aditivos.
- `OSLOG_ENABLED`, `OSLOG_FILES`, `OSLOG_WIN_CHANNELS`, `OSLOG_CURSOR_PATH`, `OSLOG_BATCH_LINES` e `OSLOG_MAX_BYTES` seguem validos.
- `OSLOG_WIN_PROVIDERS` e `OSLOG_WIN_EVENT_IDS` restringem EventLog Windows de forma aditiva; vazios por padrao.
- `OSLOG_UDP_ADDR` e `OSLOG_TCP_ADDR` habilitam listeners locais opcionais; vazios por padrao.
- `OSLOG_JOURNALD_ENABLED`, `OSLOG_JOURNALD_UNITS` e `OSLOG_JOURNALD_PRIORITIES` habilitam journald opcional em POSIX; desligado por padrao.
- `OSLOG_PROCESSORS_JSON` recebe um array JSON de processors opcionais; vazio por padrao.
- `OSLOG_DUAL_SHIP_ENDPOINTS` recebe lista CSV de endpoints internos `/v1/logs/*`; vazio por padrao.

## Evidencia incremental

- Em 2026-06-19, `go test ./internal/platform/collectors/oslogs` passou cobrindo restart sem duplicar, truncamento e rotacao de arquivo comum.
- Em 2026-06-19, a compilacao Windows do coletor passou com `GOOS=windows GOARCH=amd64 go test -c -o /tmp/aiceberg_oslogs_windows.test.exe ./internal/platform/collectors/oslogs`.
- Evidencia real PKG-69 validou EventLog em Windows desktop real, journald/syslog em Linux real, logs de container Docker e replay sem duplicidade apos reboot real.
- Em 2026-06-19, teste controlado cobriu Graylog/GELF, Linux auth, app JSON, log texto comum, classificacao por basename e parsing/classificacao de Windows Security e Sysmon no build Windows.
- Bundle auditavel: `docs/evidence/pkg60/controlled-20260619T172555Z/evidence.md`, gerado por `scripts/pkg60_logs_controlled_evidence.sh` com manifest, proveniencia e artefato bruto leve.
- Em 2026-06-19, o bundle `docs/evidence/pkg60/real-os-files-20260619T174357Z/evidence.md` validou `pkg60-real-os-files`: Windows EventLog real reaproveitado do PKG-69, Linux syslog real via `/var/log/messages` em VMAIPROD2 e arquivo comum temporario no mesmo coletor.
- Em 2026-06-19, o bundle `docs/evidence/pkg60/real-source-formats-20260619T175420Z/evidence.md` validou `pkg60-real-source-formats`: Graylog/GELF, Linux auth em `/var/log/secure`, app JSON com redaction, log texto e Windows Security real via WinRM read-only.
- Em 2026-06-19, o bundle `docs/evidence/pkg60/real-journald-windows-channels-20260619T180723Z/evidence.md` validou `pkg60-real-journald-windows-channels`: journald real em VMAIPROD2 com `source_tool=journald`/`transport=agent_journald` e Windows Security/System/Application/Sysmon real em GOVI, com Sysmon temporario removido ao final.
- Gate de lacunas: `scripts/pkg60_logs_evidence_gap_report.sh docs/evidence/pkg60`. O gate diferencia evidencia controlada de evidencia real e o fechamento aceito exige `PKG60_GAP_REPORT_REQUIRE_ACCEPTED=true PKG60_ACCEPT_CLOSURE=true`.

## Limites

- Listener TCP/UDP local esta disponivel em POSIX e desligado por padrao; Windows fica pendente de implementacao/validacao propria.
- OTLP logs ficam para PKG-62/PKG-67.
- Logs Docker/containerd e Kubernetes ficam para PKG-64/PKG-65.
- PKG-60 foi fechado por evidencia operacional com 3/3 bundles reais aceitos; novas fontes de logs devem manter bundles equivalentes antes de declarar cobertura real.

## Rollback

Desativar coleta de logs com `OSLOG_ENABLED=false` ou remover flags remotas `OSLogFiles`/`OSLogWinChannels`. Se houver regressao no binario, publicar a versao anterior do agente.

## Validacao local

```bash
go test ./internal/platform/collectors/oslogs ./internal/common/config ./internal/domain/usecase
GOOS=windows GOARCH=amd64 go test ./internal/platform/collectors/oslogs
scripts/pkg60_logs_evidence_gap_report.sh docs/evidence/pkg60
```
