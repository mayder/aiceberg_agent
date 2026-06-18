# PKG-67 - Fleet, rollout, rollback e flare seguro

## Escopo entregue

O agente expõe evidências de frota no snapshot sanitizado de runtime e aceita o comando permitido `collect_support_flare`.

Campos principais em `fleet_runtime`:

- `agent_version`;
- `goos` e `goarch`;
- `mode`;
- `config_version`;
- `config_hash`;
- `config_drift_status`;
- `auto_update_enabled`;
- `rollback_state`.

## Rollout canario

Politica recomendada para backend/orquestrador:

```json
{
  "scope": {"client_id": 1, "agent_ids": [10, 11]},
  "target_version": "x.y.z",
  "window": {"start": "2026-06-18T22:00:00Z", "end": "2026-06-19T02:00:00Z"},
  "canary": {"min_agents": 1, "max_percent": 10},
  "rollback_version": "x.y.previous",
  "expires_at": "2026-06-19T02:00:00Z"
}
```

O agente atual continua recebendo `update.version/url/sha256/force` por `/v1/agent/config`. A decisao de liberar onda global, canario ou rollback fica no backend e deve usar `update-report` como trilha. Payload sensivel de config/update deve vir assinado conforme `docs/pkg68_security_hardening.md`, com escopo, `key_id`, `signed_at` e `expires_at`.

## Rollback

O auto-update ja persiste `.pending_update.json` no diretorio de updates. O snapshot `fleet_runtime.rollback_state` permite diagnosticar versao alvo, versao anterior, artefato baixado, hash e retorno do launcher.

Rollback operacional:

1. Publicar artefato anterior conhecido e hash SHA256.
2. Enviar `update` apenas para escopo canario.
3. Acompanhar `precheck`, `download`, `validation`, `apply`, `restart`, `reconnect` e `version_confirmed`.
4. Expandir somente se o canario fechar sem falha.

## Flare seguro

Comando remoto permitido: `collect_support_flare`.

O resultado usa o mesmo canal de self-heal e aplica redaction recursiva em chaves e valores com:

- `token`;
- `secret`;
- `password`;
- `authorization`;
- `cookie`.

O flare nao executa shell nem coleta arquivo bruto arbitrario. A coleta offline pode ser feita salvando o resultado do snapshot/flare em arquivo local pelo operador.

## Rollback do recurso

Desabilitar envio de comando `collect_support_flare` no backend ou remover o comando da allowlist. Nao ha SQL no agente.

## Validacao local

```bash
go test ./internal/domain/channel ./internal/domain/usecase ./internal/bootstrap
```
