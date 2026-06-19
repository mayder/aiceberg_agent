# PKG-71 - Integracoes avancadas

## Objetivo

Evoluir `localchecks` para um ecossistema homologavel de integracoes locais sem criar execucao remota insegura e sem duplicar Agentless.

## Estados de integracao

- `official`: mantida no core, com manifest, teste e rollback.
- `beta`: pode entrar em canario com permissao minima e credencial por referencia.
- `experimental`: nao deve ser ativada amplamente sem homologacao real.

Integrações `beta` e `experimental` não executam por padrão, mesmo se aparecerem em `local_checks`. Para execução produtiva mínima, o check precisa declarar:

```json
"config": {
  "homologation_status": "approved",
  "homologation_ref": "ticket-ou-evidencia"
}
```

Sem esses campos, o agente não abre conexão com o alvo e reporta `service_check.status=blocked` com `reason=integration_not_homologated`.

## OpenMetrics

`openmetrics` agora suporta:

- `metric_allowlist`: lista CSV com nome exato ou prefixo `*`, por exemplo `http_*`;
- `label_allowlist`: lista CSV de labels permitidas;
- `max_metrics`: limite de series emitidas por scrape, padrao 200, maximo 1000;
- `max_label_values`: limite de valores distintos por label no scrape, padrao 50, maximo 500.

O parser preserva labels permitidas e descarta labels fora da allowlist ou acima do limite de cardinalidade.

## JMX/JVM

JMX nao embute dependencia Java nem executa comando local. O modo suportado neste pacote e `mode=jolokia`, por HTTP/HTTPS, coletando quando presentes:

- `jvm.heap.used`;
- `jvm.heap.max`;
- `jvm.threads.count`;
- `jvm.gc.collection_count`;
- `jvm.gc.collection_time_ms`.

Credenciais devem ser passadas por `credentials_ref`, nunca inline.

## WMI/Windows

`iis_wmi` e `windows_service` continuam catalogados como `experimental` ate validacao em Windows Server. A execucao exige `homologation_status=approved` e `homologation_ref`.

`iis_wmi` coleta counters locais por PowerShell fixo em Windows:

- `windows.memory.free_kb`;
- `windows.memory.total_kb`;
- `iis.current_connections`.

`windows_service` consulta um servico local por `service_name`/`target` validado e emite `windows.service.running`.

Permissoes minimas esperadas:

- usuario local com leitura de WMI/CIM basica;
- permissao de `Get-Counter` para IIS quando IIS estiver habilitado;
- permissao de `Get-Service` para o servico configurado;
- sem credencial inline, sem shell remoto e sem script arbitrario vindo da configuracao.

Fora de Windows, essas integracoes retornam `service_check.status=skipped` com `reason=windows_only`.

## Bancos, fila e web servers

`redis` esta como oficial para reachability TCP e `integration.reachable`. PostgreSQL/MySQL/SQL Server e RabbitMQ seguem beta por enquanto, apenas reachability segura por TCP e exigindo homologacao minima. Nginx/Apache usam HTTP check. IIS usa a base Windows experimental descrita acima.

## Manifestos oficiais

- `integrations/localchecks/manifests/openmetrics.json`;
- `integrations/localchecks/manifests/redis.json`.

Cada manifest declara versao, status, config schema, permissoes, metricas, riscos e rollback.

## Validacao local realizada

- `go test ./internal/platform/collectors/localchecks`

Cobertura:

- OpenMetrics com allowlist de metricas e labels;
- limite de cardinalidade de labels;
- JMX via fixture Jolokia;
- metadados de integracao oficial em Redis/TCP;
- bloqueio de integracao beta/experimental sem homologacao minima;
- manifest de integracao criada pelo guia aceito quando usa kind e permissoes seguras;
- bloqueio de tipo arbitrario e redaction herdados do PKG-66.

Revalidacao em 2026-06-19 na branch `mayder/agente-datadog-paridade`:

- `go test ./internal/platform/collectors/localchecks`
- `go test ./internal/platform/collectors/containers ./internal/platform/collectors/kubernetes`
- `vendor/bin/codecept run -c common unit services/agent/AgentLocalCheckStatusServiceTest.php`

Todos passaram. Isso nao fecha homologacao real.

## Pendencias reais

- app real com `/metrics`;
- app Java real com Jolokia e credencial minima;
- Windows Server para WMI/IIS/Windows Service;
- PostgreSQL/MySQL/Redis reais com credenciais minimas;
- execucao real de checks descobertos por autodiscovery em Docker/Kubernetes controlados;
- UI/marketplace interno no web.

## Rollback

Desativar a integracao especifica em `local_checks` ou desligar `local_checks_enabled=false`. Nao ha SQL neste pacote.
