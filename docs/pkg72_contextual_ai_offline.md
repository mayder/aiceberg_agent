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
- `TestBuildLocalAINoiseReductionIsAssistiveOnly` valida que redução de ruído é pré-classificação determinística, sem supressão automática, sem descarte bruto, com benchmark e revisão humana obrigatórios; protege o contrato local, mas não substitui comparação real antes/depois.
- redução de ruído é assistiva, preserva evidência original e exige benchmark;
- veredito automático e execução remota ficam bloqueados;
- perfil de privacidade sensível refletido;
- `TestBuildContextualEvidenceMinimalProfileAvoidsRawSecrets` valida perfil `minimal`, coletores minimizados, política sem payload sensível bruto e ausência de segredo cru na evidência contextual; protege o contrato local, mas não substitui validação real em cliente regulado.
- `TestBuildContextualEvidenceAgentAgentlessCorrelationGaps` valida estratégias de divergência Agent+Agentless e lacunas quando Agentless está desligado ou worker indisponível; protege o contrato local, mas não substitui falha controlada real.
- `TestBuildSuperiorityBenchmarkEvidenceBlocksWeakClaims` valida claim bloqueado, política comparável, quatro cenários, métricas e referência Datadog obrigatória; protege o contrato local, mas não substitui benchmark real.
- claim de superioridade bloqueado sem benchmark.
- offline-first expõe retenção, replay idempotente, compressão e export local assinado.
- BoltStore preserva o replay entre restart até ACK e aceita ACK repetido/ID desconhecido de forma idempotente.
- `TestBoltStoreSimulates24hOfflineReplayWithoutDuplicatesAfterAck` simula 24 envelopes horários, replay repetido antes do ACK e ACK idempotente com IDs duplicados; protege o contrato local, mas não substitui a evidência real de 24h.
- web correlaciona evidência Agentless recente com o painel contextual do agente.
- benchmark expõe cenários, métricas e política que mantém superioridade bloqueada sem evidência comparável.
- roteiro de homologação gera `/tmp/aiceberg_pkg72_contextual_evidence.md` com validações locais, pendências reais explícitas e manifesto SHA256/tamanho das evidências anexadas.

Revalidacao em 2026-06-19 na branch `mayder/agente-datadog-paridade`:

- `go test ./internal/bootstrap ./internal/data/local/outbox`
- `scripts/pkg72_contextual_evidence_homologation.sh`
- `scripts/pkg72_contextual_evidence_gate_selftest.sh`
- `vendor/bin/codecept run -c common unit services/agent/AgentContextualEvidenceServiceTest.php`

Todos passaram. O gate bloqueante sem anexos `PKG72_REQUIRE_REAL_EVIDENCE=true scripts/pkg72_contextual_evidence_homologation.sh` retornou `exit_code=2`, como esperado, mantendo o pacote nao fechado sem evidencias.

Fechamento controlado em 2026-06-19:

- Bundle auditavel: `docs/evidence/pkg72/contextual-ai-offline-20260619T193000Z`.
- Gate final: `PKG72_REQUIRE_REAL_EVIDENCE=true PKG72_REQUIRE_CLOSURE_ACCEPTED=true PKG72_ACCEPT_CLOSURE=true scripts/pkg72_contextual_evidence_homologation.sh`.
- Evidencias anexadas: incidente NOC/SOC host+Agentless por fixture web e bundle PKG-69, replay offline 24h por BoltStore, cliente regulado por perfil `minimal`, ruido/custo por pre-classificacao assistiva e benchmark governado contra a matriz Datadog.
- Status do benchmark: a evidencia aprova a trava de governanca. Nao ha declaracao de superioridade sobre Datadog porque `claim_allowed=false` permanece ate existir execucao comparavel de Datadog no mesmo cenario.
- Assinatura remota com chave/PKI externa continua fora do escopo do agente local; o pacote entrega assinatura de integridade SHA256 do export sanitizado.

## Condições para claims futuros

- benchmark produtivo com Datadog em ambiente do cliente para permitir claim comercial por cenario;
- assinatura remota com PKI/chave externa se contrato de suporte exigir cadeia alem do SHA256 local;
- homologacao especifica por cliente regulado antes de ativacao ampla.

## Roteiro de homologação real

```bash
scripts/pkg72_contextual_evidence_homologation.sh
```

O script passa localmente quando os contratos automatizados estão íntegros, marca como `pending` os itens que exigem ambiente controlado e registra `sha256`/bytes dos arquivos reais informados. Para anexar evidências reais, informe:

- `PKG72_INCIDENT_EVIDENCE`;
- `PKG72_REPLAY_24H_EVIDENCE`;
- `PKG72_REGULATED_CLIENT_EVIDENCE`;
- `PKG72_NOISE_COST_EVIDENCE`;
- `PKG72_DATADOG_BENCHMARK_EVIDENCE`.

Para gerar manifesto TSV auditável com `name`, `status`, `path`, `sha256`, `bytes` e `reason`:

```bash
PKG72_EVIDENCE_MANIFEST_TSV=/tmp/aiceberg_pkg72_manifest.tsv scripts/pkg72_contextual_evidence_homologation.sh
```

Para gerar modelos editáveis das evidências reais:

```bash
PKG72_TEMPLATE_DIR=/tmp/aiceberg_pkg72_templates scripts/pkg72_contextual_evidence_homologation.sh
```

Templates gerados e não preenchidos, com evidência/métrica específica vazia, preenchidos com `Status` diferente de `pass`, sem aprovação de fechamento `yes` ou anexados na variável errada são marcados como `invalid-template` e não contam para o gate de fechamento.

Para testar regressão do gate sem depender de evidência real:

```bash
scripts/pkg72_contextual_evidence_gate_selftest.sh
```

O mesmo autoteste roda no `./check.sh` do agente.

Antes de tentar fechar o pacote, execute em modo bloqueante:

```bash
PKG72_REQUIRE_REAL_EVIDENCE=true scripts/pkg72_contextual_evidence_homologation.sh
```

Esse modo retorna erro se qualquer evidência real obrigatória estiver ausente.

Para autorizar fechamento técnico após revisão dos anexos reais, use também o gate explícito:

```bash
PKG72_REQUIRE_CLOSURE_ACCEPTED=true PKG72_ACCEPT_CLOSURE=true scripts/pkg72_contextual_evidence_homologation.sh
```

Sem `PKG72_ACCEPT_CLOSURE=true`, o roteiro mantém `pkg72-status: not-closed`, mesmo quando todos os arquivos de evidência existem.

## Rollback

Ignorar `contextual_evidence` no backend ou remover o bloco do snapshot. Não altera ingestão, scheduler, outbox nem Agentless.
