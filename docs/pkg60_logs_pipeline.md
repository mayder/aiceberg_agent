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

- Unix-like: leitura por arquivo com cursor persistente, reset seguro quando o arquivo e truncado, agrupamento multiline simples e parser syslog existente.
- Windows: leitura por canais EventLog via `wevtutil`, cursor por `Record ID`, metadados de canal/provider/evento e redaction antes do envio.
- Local POSIX: listeners opcionais TCP/UDP recebem linhas de aplicacoes locais, aplicam os mesmos limites, filtros e redaction, e entram no mesmo `/v1/logs/raw`.
- JSON: campos JSON de primeiro nivel sao copiados para `attributes`; chaves sensiveis sao mascaradas.
- Redaction: token, senha, secret, API key, cookie e `Authorization` sao mascarados antes de sair do host.
- Filtros: `OSLOG_INCLUDE_REGEX`, `OSLOG_EXCLUDE_REGEX` e `OSLOG_MIN_SEVERITY` podem ser definidos por env ou config remota em `logs.include_regex`, `logs.exclude_regex` e `logs.min_severity`.

## Compatibilidade

- `/v1/logs/raw` nao muda.
- `events[]` continua existindo.
- Campos novos sao aditivos.
- `OSLOG_ENABLED`, `OSLOG_FILES`, `OSLOG_WIN_CHANNELS`, `OSLOG_CURSOR_PATH`, `OSLOG_BATCH_LINES` e `OSLOG_MAX_BYTES` seguem validos.
- `OSLOG_UDP_ADDR` e `OSLOG_TCP_ADDR` habilitam listeners locais opcionais; vazios por padrao.

## Limites

- Listener TCP/UDP local esta disponivel em POSIX e desligado por padrao; Windows fica pendente de implementacao/validacao propria.
- Dual-shipping e OTLP logs ficam para PKG-62/PKG-67.
- Journald nativo, logs Docker/containerd e Kubernetes ficam para PKG-64/PKG-65.
- Validacao operacional real Windows/Linux/container/proxy/disco cheio fica para PKG-69.

## Rollback

Desativar coleta de logs com `OSLOG_ENABLED=false` ou remover flags remotas `OSLogFiles`/`OSLogWinChannels`. Se houver regressao no binario, publicar a versao anterior do agente.

## Validacao local

```bash
go test ./internal/platform/collectors/oslogs ./internal/common/config ./internal/domain/usecase
GOOS=windows GOARCH=amd64 go test ./internal/platform/collectors/oslogs
```
