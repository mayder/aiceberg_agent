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

## Limites

- Listener TCP/UDP local esta disponivel em POSIX e desligado por padrao; Windows fica pendente de implementacao/validacao propria.
- OTLP logs ficam para PKG-62/PKG-67.
- Logs Docker/containerd e Kubernetes ficam para PKG-64/PKG-65.
- Graylog como transporte, Windows Security completo, Linux auth e Sysmon ainda precisam de evidencia real dedicada antes de fechar PKG-60 como 100%.

## Rollback

Desativar coleta de logs com `OSLOG_ENABLED=false` ou remover flags remotas `OSLogFiles`/`OSLogWinChannels`. Se houver regressao no binario, publicar a versao anterior do agente.

## Validacao local

```bash
go test ./internal/platform/collectors/oslogs ./internal/common/config ./internal/domain/usecase
GOOS=windows GOARCH=amd64 go test ./internal/platform/collectors/oslogs
```
