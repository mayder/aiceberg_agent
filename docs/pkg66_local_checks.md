# PKG-66 - Runtime de checks/plugins locais

## Escopo entregue

O agente possui coletor `localchecks`, desligado por padrao, para executar checks locais por allowlist de tipos. Nao existe shell generico, comando arbitrario nem execucao remota.

Config local:

```env
LOCAL_CHECKS_ENABLED=true
LOCAL_CHECKS_INTERVAL=30
LOCAL_CHECKS_MAX_CHECKS=100
LOCAL_CHECKS_MAX_BYTES=1048576
LOCAL_CHECKS_MANIFEST_DIRS=./integrations/localchecks/manifests,/etc/aiceberg/localchecks.d
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
    "manifest_dirs": ["/etc/aiceberg/localchecks.d"],
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
- `integrations`: manifests locais carregados, com kind, version, status, permissions e rollback.

`credentials_ref` nunca envia o valor bruto; o payload usa `[redacted-ref]`.

## Tipos permitidos

- `http`: GET com status HTTP e service check;
- `tcp`: conexao TCP simples;
- `openmetrics`: scrape HTTP e parser basico de exposition format;
- `jmx`, `postgresql`, `mysql`, `sqlserver`, `redis`, `rabbitmq`, `nginx`, `apache`, `iis_wmi`, `windows_service`: check basico por HTTP/TCP, sem credencial inline e sem shell remoto.

JMX, bancos, IIS/WMI e Windows Service entram como base segura. Coletores profundos por protocolo nativo devem ser evoluidos em integracoes oficiais com manifest e testes especificos.
Integrações `beta` ou `experimental` só executam quando o check inclui `config.homologation_status=approved` e `config.homologation_ref` preenchido; caso contrario o agente retorna `service_check.status=blocked` sem abrir conexão.

Catalogo inicial: `integrations/localchecks/catalog.json`.

## Instalação e remoção de integrações

Integrações instaláveis sem rebuild usam manifests JSON em diretórios controlados por `LOCAL_CHECKS_MANIFEST_DIRS`.

- instalar: adicionar um arquivo `*.json` com `kind`, `version`, `status`, `permissions` e `rollback`;
- remover: apagar o manifest e remover/desabilitar checks que usem o `kind`;
- o agente não executa código do manifest;
- manifests com kind fora da allowlist ou permissões contendo shell/exec/command são ignorados.

## Segurança

- Shell, script, powershell, bash, cmd e comandos arbitrarios nao sao tipos permitidos.
- Cada check executa com timeout proprio.
- Target com query string e erros com `token=`, `secret=`, `password=`, `api_key=` ou similares sao mascarados.
- Resultados usam limites de quantidade e bytes.
- Falha de um check nao trava o agente.
- Diretórios de manifests vindos por config remota são tratados como configuração sensível e devem seguir assinatura.

## Autodiscovery container/Kubernetes

PKG-64 e PKG-65 materializam templates em `body.containers.autodiscovery_checks` e `body.kubernetes.autodiscovery_checks`. Cada item e normalizado para o contrato de `local_checks` com `kind`, `target`, `enabled` e `tags`, mantendo os metadados originais de container/pod para auditoria.

A ponte aceita apenas tipos do allowlist local (`http`, `openmetrics`, `tcp`, `redis`, `postgresql`, `mysql`, `sqlserver`, `rabbitmq`, `nginx`, `apache`) e nao executa shell remoto.

## Rollback

Definir `LOCAL_CHECKS_ENABLED=false` ou config remota `local_checks.enabled=false`. Nao ha SQL.

## Validacao local

```bash
go test ./internal/platform/collectors/localchecks ./internal/common/config ./internal/domain/usecase ./internal/bootstrap
```

## Evidencia de fechamento

Bundle aceito: `docs/evidence/pkg66/localchecks-lifecycle-rollback-upgrade-20260619T185846Z`.

Cobertura:

- criacao e execucao de check HTTP OK;
- falha de check HTTP registrada sem travar o agente;
- tipo fora da allowlist bloqueado sem executar shell;
- rollback por config remota `local_checks.enabled=false`;
- upgrade de manifest instalavel mantendo a configuracao dos checks;
- `credentials_ref` e segredo em target redigidos no payload.
