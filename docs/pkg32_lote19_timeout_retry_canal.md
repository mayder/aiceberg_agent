# PKG-32 Lote 19 - Timeout, cancelamento cooperativo e retry

## Escopo

O agente aplica timeout por comando recebido pelo canal operacional e usa o mesmo `command_id` como chave idempotente entre canal, retry interno e polling legado.

## Politica

- `timeout_ms` pode vir no envelope ou no payload do comando.
- Sem `timeout_ms`, o agente usa timeout padrao de 5 minutos.
- O contexto da tentativa expira no timeout e e propagado para as dependencias do comando.
- Retry so ocorre quando o comando vier marcado como `retryable=true` ou `retry.enabled=true`.
- `max_attempts` e limitado localmente a 5 tentativas.
- `retry_after_ms` controla a espera entre tentativas; sem valor, usa 2 segundos.

## Eventos reportados

- `ack`: comando aceito ou duplicado.
- `progress`: tentativa em execucao.
- `timeout`: tentativa expirou por timeout.
- `retry`: proxima tentativa agendada.
- `error`: falha final.
- `result`: resultado final com evidencia sanitizada.

## Dedupe

O `command_id` e marcado antes da primeira tentativa. Retries internos nao reabrem a dedupe e entregas repetidas pelo canal ou pelo polling retornam `duplicate` sem executar o corpo do comando novamente.

## Rollback

Rollback de codigo: remover o uso de `ExecuteChannelSelfHealCommand` no handler do canal e voltar para `ExecuteSelfHealOnce`. Nao ha SQL nem estado persistente novo neste lote.
