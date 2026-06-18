# PKG-66 - Runtime de checks/plugins locais

## Escopo entregue

O agente possui coletor `localchecks`, desligado por padrao, para executar checks locais por allowlist de tipos. Nao existe shell generico, comando arbitrario nem execucao remota.

Config local:

```env
LOCAL_CHECKS_ENABLED=true
LOCAL_CHECKS_INTERVAL=30
LOCAL_CHECKS_MAX_CHECKS=100
LOCAL_CHECKS_MAX_BYTES=1048576
LOCAL_CHECKS_JSON='[{"id":"api","kind":"http","target":"https://app.local/health","timeout_ms":3000,"enabled":true}]'
```

Config remota:

```json
{
  "local_checks": {
    "enabled": true,
    "interval": 30,
    "max_checks": 100,
    "max_bytes": 1048576,
    "checks": [
      {
        "id": "api-health",
        "kind": "http",
        "version": "1",
        "interval": 30,
        "timeout_ms": 3000,
        "tags": ["service:api", "env:prod"],
        "target": "https://app.local/health",
        "enabled": true
      }
    ]
  }
}
```

## Contrato

O coletor envia `body.local_checks` para `/v1/ingest/metrics`:

- `check_id`, `kind`, `version`, `interval`, `timeout_ms`, `tags`, `target`, `credentials_ref`;
- `result`: `ok` ou `failed`;
- `metrics`: metricas extraidas pelo check;
- `logs`: mensagens curtas e sanitizadas quando aplicavel;
- `service_check`: status operacional padronizado;
- `error`: erro sanitizado quando houver falha.

`credentials_ref` nunca envia o valor bruto; o payload usa `[redacted-ref]`.

## Tipos permitidos

- `http`: GET com status HTTP e service check;
- `tcp`: conexao TCP simples;
- `openmetrics`: scrape HTTP e parser basico de exposition format;
- `jmx`, `postgresql`, `mysql`, `redis`, `nginx`, `apache`, `iis_wmi`, `windows_service`: check basico por HTTP/TCP, sem credencial inline e sem shell.

JMX, bancos, IIS/WMI e Windows Service entram como base segura. Coletores profundos por protocolo nativo devem ser evoluidos em integracoes oficiais com manifest e testes especificos.

Catalogo inicial: `integrations/localchecks/catalog.json`.

## Segurança

- Shell, script, powershell, bash, cmd e comandos arbitrarios nao sao tipos permitidos.
- Cada check executa com timeout proprio.
- Target com query string e erros com `token=`, `secret=`, `password=`, `api_key=` ou similares sao mascarados.
- Resultados usam limites de quantidade e bytes.
- Falha de um check nao trava o agente.

## Autodiscovery Kubernetes

O PKG-65 materializa templates em `body.kubernetes.autodiscovery_checks`. O PKG-66 define o contrato local, mas a ponte automatica entre annotations e execucao fica pendente para evolucao segura com escopo/assinatura.

## Rollback

Definir `LOCAL_CHECKS_ENABLED=false` ou config remota `local_checks.enabled=false`. Nao ha SQL.

## Validacao local

```bash
go test ./internal/platform/collectors/localchecks ./internal/common/config ./internal/domain/usecase ./internal/bootstrap
```
