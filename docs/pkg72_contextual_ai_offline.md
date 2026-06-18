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
- `superiority_benchmark`: trava explícita para impedir claim sem evidência, cenários comparáveis e métricas obrigatórias.

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
- contrato de replay até ACK, ACK idempotente e escopo de retry por rota/identidade;
- modo HUB/relay;
- proxy configurado;
- export de suporte via flare sanitizado;
- assinatura de integridade SHA256 sobre evidência offline sanitizada.

A assinatura local é controle de integridade do snapshot sanitizado. Ela não substitui assinatura remota com chave/PKI nem libera claim de superioridade.

Em modo `relay`, o snapshot marca `relay_to_hub_only=true` e `direct_api_from_relay=false`: o replay preserva a topologia relay -> HUB -> AIceberg e não abre envio direto do relay para a API.

## Agent + Agentless

A estratégia documentada no snapshot cobre:

- host saudável, rede falhando;
- rede OK, serviço local falhando;
- agente recente, SNMP atrasado.

O web correlaciona as últimas `asset_observation` Agentless vinculadas ao agente/HUB para SNMP, ICMP, TCP, TLS, HTTP e HTTPS. A validação real de falha cruzada em ambiente controlado fica pendente.

## Benchmark

`superiority_benchmark.claim_allowed=false` permanece até existir comparação objetiva. O snapshot expõe `status=pending_evidence`, política que bloqueia superioridade sem benchmark e cenários mínimos:

- `noc_soc_context`: tempo de diagnóstico, completude da evidência e passos do operador;
- `sovereign_offline`: sucesso de replay offline, taxa de duplicação e integridade do export de suporte;
- `agent_plus_agentless`: correlação detectada, falso positivo e vínculo com observação Agentless;
- `noise_reduction`: ruído antes/depois e revisão manual obrigatória.

Cada cenário exige referência Datadog comparável e evidência bruta rastreável antes de qualquer declaração de superioridade.

## Validação local realizada

- `go test ./internal/bootstrap`
- `go test ./internal/data/local/outbox`
- `scripts/pkg72_contextual_evidence_homologation.sh`
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
- BoltStore preserva o replay entre restart até ACK e aceita ACK repetido/ID desconhecido de forma idempotente.
- web correlaciona evidência Agentless recente com o painel contextual do agente.
- benchmark expõe cenários, métricas e política que mantém superioridade bloqueada sem evidência comparável.
- roteiro de homologação gera `/tmp/aiceberg_pkg72_contextual_evidence.md` com validações locais e pendências reais explícitas.

## Pendências reais

- incidente NOC/SOC real com evidência host + agentless em falha controlada;
- replay 24h offline sem duplicação relevante;
- assinatura remota com chave/PKI, se for exigida para suporte externo;
- cliente regulado com coleta reduzida;
- comparação de ruído/custo antes e depois;
- benchmark com Datadog por cenário;
- tela web exibindo evidência e lacunas.

## Roteiro de homologação real

```bash
scripts/pkg72_contextual_evidence_homologation.sh
```

O script passa localmente quando os contratos automatizados estão íntegros e marca como `pending` os itens que exigem ambiente controlado. Para anexar evidências reais, informe:

- `PKG72_INCIDENT_EVIDENCE`;
- `PKG72_REPLAY_24H_EVIDENCE`;
- `PKG72_REGULATED_CLIENT_EVIDENCE`;
- `PKG72_NOISE_COST_EVIDENCE`;
- `PKG72_DATADOG_BENCHMARK_EVIDENCE`.

## Rollback

Ignorar `contextual_evidence` no backend ou remover o bloco do snapshot. Não altera ingestão, scheduler, outbox nem Agentless.
