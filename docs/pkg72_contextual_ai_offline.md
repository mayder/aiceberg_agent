# PKG-72 - Diferenciais AIceberg

## Objetivo

Adicionar uma base local para diferenciais AIceberg sem declarar superioridade global sobre Datadog. A entrega deste pacote expõe evidência contextual, pré-classificação determinística, política offline e correlação agent + agentless no snapshot seguro do agente.

## Contrato

`inspect_runtime_config` e `collect_support_flare` passam a incluir `contextual_evidence`.

Campos principais:

- `host_evidence`: fontes resumidas, lacunas e política de anexo NOC/SOC;
- `local_ai`: regras determinísticas locais, política de redução de ruído, bloqueio de veredito automático, sem LLM obrigatório e sem ação destrutiva;
- `offline_first`: outbox, idempotência, compressão, retenção local, HUB/relay/proxy e export local assinado por integridade;
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

`noise_reduction` é assistivo: mantém evidência original, não derruba eventos brutos, não faz supressão automática e exige benchmark antes de qualquer claim. `verdict_policy.automatic_verdict=false`, `destructive_action=false` e bloqueio de `execute_command`, fechamento de incidente, mudança de threshold e declaração de superioridade são parte do contrato.

## Privacidade

Variáveis locais:

- `PRIVACY_PROFILE=standard|sensitive|minimal`;
- `SENSITIVE_MODE=true|false`.

O snapshot expõe apenas estado e lacunas. Não inclui senha, token, payload completo, comando bruto ou decisão destrutiva.

## Offline-first

O agente já usa outbox local e idempotência HTTP. O bloco `offline_first` torna isso auditável no snapshot com:

- path configurado da outbox;
- limite local em MB da outbox do agente e do agentless;
- idempotência HTTP para replay;
- suporte a compressão quando `HTTP_GZIP=true`;
- política local de retenção e flush;
- modo HUB/relay;
- proxy configurado;
- export de suporte via flare sanitizado;
- assinatura de integridade SHA256 sobre evidência offline sanitizada.

A assinatura local é controle de integridade do snapshot sanitizado. Ela não substitui assinatura remota com chave/PKI nem libera claim de superioridade.

## Agent + Agentless

A estratégia documentada no snapshot cobre:

- host saudável, rede falhando;
- rede OK, serviço local falhando;
- agente recente, SNMP atrasado.

O web correlaciona as últimas `asset_observation` Agentless vinculadas ao agente/HUB para SNMP, ICMP, TCP, TLS, HTTP e HTTPS. A validação real de falha cruzada em ambiente controlado fica pendente.

## Validação local realizada

- `go test ./internal/bootstrap`
- `./check.sh`

Cobertura:

- `contextual_evidence` presente no snapshot;
- IA local não exige LLM;
- decisão destrutiva bloqueada;
- redução de ruído é assistiva, preserva evidência original e exige benchmark;
- veredito automático e execução remota ficam bloqueados;
- perfil de privacidade sensível refletido;
- claim de superioridade bloqueado sem benchmark.
- offline-first expõe retenção, replay idempotente, compressão e export local assinado.
- web correlaciona evidência Agentless recente com o painel contextual do agente.

## Pendências reais

- incidente NOC/SOC real com evidência host + agentless em falha controlada;
- replay 24h offline sem duplicação relevante;
- assinatura remota com chave/PKI, se for exigida para suporte externo;
- cliente regulado com coleta reduzida;
- comparação de ruído/custo antes e depois;
- benchmark com Datadog por cenário;
- tela web exibindo evidência e lacunas.

## Rollback

Ignorar `contextual_evidence` no backend ou remover o bloco do snapshot. Não altera ingestão, scheduler, outbox nem Agentless.
