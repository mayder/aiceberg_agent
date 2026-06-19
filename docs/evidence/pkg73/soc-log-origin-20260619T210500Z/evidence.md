# PKG-73 - Evidencia controlada de taxonomia SOC de logs

Data UTC: 2026-06-19T21:05:00Z

## Escopo validado

- Normalizador comum `internal/common/soclog`.
- `oslogs` POSIX: Graylog/GELF, Linux auth, app JSON, journald e processors.
- Windows EventLog: compilacao cruzada e testes de parser em build Windows para Security, Sysmon, Application e DistributedCOM.
- Docker logs: contrato SOC e override por label.
- Kubernetes pod logs: contrato SOC e override por annotation.
- OTLP logs: contrato SOC, redaction e descarte de evento sem severidade quando `OSLOG_MIN_SEVERITY` esta configurado.
- Web `/v1/logs/raw`: snapshot parseavel `AICEBERG_SOC_ORIGIN=` para PKG-54, payload novo e legado, e bloqueio de falso positivo DistributedCOM.

## Validacoes executadas

```text
go test ./internal/common/soclog ./internal/platform/collectors/oslogs ./internal/platform/collectors/containers ./internal/platform/collectors/kubernetes ./internal/platform/collectors/otlp
GOOS=windows GOARCH=amd64 go test -c ./internal/platform/collectors/oslogs -o /tmp/aiceberg-oslogs-windows.test.exe
php -l common/services/soc/SocAgentLogOriginNormalizerService.php
php -l common/tests/unit/services/soc/SocAgentLogOriginNormalizerServiceTest.php
php -l api/modules/v1/controllers/LogsController.php
vendor/bin/codecept run -c common unit services/soc/SocAgentLogOriginNormalizerServiceTest.php
./check.sh no aiceberg_agent
./check.sh no aiceberg_web
```

## Resultado

- Eventos security reais/estruturados ficam elegiveis a SOC com motivo auditavel.
- Eventos operacionais Windows, como DistributedCOM `10028`, permanecem observability/no.
- Logs de aplicacao, container, Kubernetes e OTLP permanecem conditional por padrao.
- Campos sensiveis nao sao promovidos.
- Backend legado continua aceito; campos novos sao aditivos.

## Limite

Evidencia controlada e compilacao cruzada Windows. Homologacao produtiva continua dependente da matriz operacional real do cliente quando houver fonte EDR/NDR/firewall real.
