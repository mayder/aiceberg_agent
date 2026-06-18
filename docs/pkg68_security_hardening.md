# PKG-68 - Seguranca, assinatura e credenciais

## Escopo entregue

O agente adiciona endurecimento compativel e opt-in para configuracao remota sensivel:

- assinatura HMAC-SHA256 de payload de `/v1/agent/config`;
- rejeicao de payload sensivel sem assinatura quando `REMOTE_CONFIG_SIGNATURE_SECRET` esta configurado;
- modo obrigatorio com `REMOTE_CONFIG_SIGNATURE_REQUIRED=true`;
- expiracao de assinatura por `signature.expires_at`;
- bloqueio de downgrade de update sem `force=true`;
- rotacao remota de token via `token_rotation.new_token`, persistida no arquivo local;
- evidencia `security_runtime` no snapshot sanitizado;
- TLS 1.2 minimo no client HTTP;
- bloqueio de `TLS_INSECURE_SKIP_VERIFY=true` contra API de producao, salvo `TLS_INSECURE_ALLOW_PROD=true`.

## Assinatura de config

Campo esperado:

```json
{
  "signature": {
    "algorithm": "hmac-sha256",
    "key_id": "fleet-key-1",
    "value": "<hex hmac sha256>",
    "signed_at": "2026-06-18T12:00:00Z",
    "expires_at": "2026-06-18T12:10:00Z",
    "scope": "client:1"
  }
}
```

O HMAC e calculado sobre o JSON canonico do payload com `signature.value` vazio. Payloads com `update`, `auto_update`, `token_rotation`, `collect_now`, `local_checks`, Docker socket ou Kubernetes token path sao tratados como sensiveis.

## Variaveis

```env
REMOTE_CONFIG_SIGNATURE_SECRET=<secret local>
REMOTE_CONFIG_SIGNATURE_REQUIRED=true
REMOTE_CONFIG_ALLOW_UNSIGNED_SENSITIVE=false
TLS_INSECURE_SKIP_VERIFY=false
TLS_INSECURE_ALLOW_PROD=false
```

## Rotacao de token

Payload remoto:

```json
{
  "token_rotation": {
    "new_token": "<novo-token>",
    "previous_expires_at": "2026-06-18T12:10:00Z",
    "reason": "regular_rotation"
  }
}
```

O token novo e persistido em `AGENT_TOKEN_PATH` ou `./data/agent.token`. O valor nunca e logado. A aplicacao no processo atual pode exigir restart controlado para todos os use cases passarem a usar o novo token em memoria.

## Limites atuais

- Assinatura assimetrica/cadeia de confianca de artefatos fica pendente.
- Rotacao com periodo de transicao e revogacao backend depende da API/web.
- Modo FIPS e pinning TLS ficam documentados como `not_claimed` ate homologacao.
- Validacao real de proxy autenticado/TLS invalido fica para PKG-69.

## Rollback

- Remover `REMOTE_CONFIG_SIGNATURE_REQUIRED`;
- definir `REMOTE_CONFIG_ALLOW_UNSIGNED_SENSITIVE=true` temporariamente;
- publicar agente anterior se a politica bloquear config valida;
- manter `TLS_INSECURE_SKIP_VERIFY=false` em producao.

## Validacao local

```bash
go test ./internal/common/config ./internal/common/httpx ./internal/domain/usecase ./internal/bootstrap
```
