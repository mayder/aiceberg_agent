# PKG-72 - Diferenciais AIceberg

## Objetivo

Adicionar uma base local para diferenciais AIceberg sem declarar superioridade global sobre Datadog. A entrega deste pacote expõe evidência contextual, pré-classificação determinística, política offline e correlação agent + agentless no snapshot seguro do agente.

## Contrato

`inspect_runtime_config` e `collect_support_flare` passam a incluir `contextual_evidence`.

Campos principais:

- `host_evidence`: fontes resumidas, lacunas e política de anexo NOC/SOC;
- `local_ai`: regras determinísticas locais, sem LLM obrigatório e sem ação destrutiva;
- `offline_first`: outbox, idempotência, HUB/relay/proxy e suporte flare;
- `privacy`: perfil `standard|sensitive|minimal`, modo sensível e coletores minimizados;
- `agent_agentless`: estratégia de correlação entre agente e agentless;
- `superiority_benchmark`: trava explícita para impedir claim sem evidência.

## IA local/governada

Não há modelo pago nem veredito automático neste pacote. A pré-classificação local é determinística:

- redaction;
- dedupe;
- pontuação de ruído;
- detecção de lacunas;
- divergência agente + agentless.

`destructive_action=false` é parte do contrato.

## Privacidade

Variáveis locais:

- `PRIVACY_PROFILE=standard|sensitive|minimal`;
- `SENSITIVE_MODE=true|false`.

O snapshot expõe apenas estado e lacunas. Não inclui senha, token, payload completo, comando bruto ou decisão destrutiva.

## Offline-first

O agente já usa outbox local e idempotência HTTP. O bloco `offline_first` torna isso auditável no snapshot com:

- path configurado da outbox;
- limite local em MB;
- idempotência HTTP;
- modo HUB/relay;
- proxy configurado;
- export de suporte via flare sanitizado.

## Agent + Agentless

A estratégia documentada no snapshot cobre:

- host saudável, rede falhando;
- rede OK, serviço local falhando;
- agente recente, SNMP atrasado.

A correlação visual no web e validação real ficam pendentes.

## Validação local realizada

- `go test ./internal/bootstrap`
- `./check.sh`

Cobertura:

- `contextual_evidence` presente no snapshot;
- IA local não exige LLM;
- decisão destrutiva bloqueada;
- perfil de privacidade sensível refletido;
- claim de superioridade bloqueado sem benchmark.

## Pendências reais

- incidente NOC/SOC real com evidência host + agentless;
- replay 24h offline sem duplicação relevante;
- cliente regulado com coleta reduzida;
- comparação de ruído/custo antes e depois;
- benchmark com Datadog por cenário;
- tela web exibindo evidência e lacunas.

## Rollback

Ignorar `contextual_evidence` no backend ou remover o bloco do snapshot. Não altera ingestão, scheduler, outbox nem Agentless.
