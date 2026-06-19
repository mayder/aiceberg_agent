# PKG-70 - Rede, USM e workload security

## Objetivo

Evoluir `networkcapture` sem quebrar `/v1/ingest/network_capture`, adicionando descoberta de servicos, dependencias de rede, resumo NPM, estado de system probe/eBPF e sinais de workload security.

## Ativacao

Tudo fica desligado por padrao para preservar instalacoes atuais.

- `network_advanced_enabled`: adiciona `body.service_map` e `body.network_performance`.
- `usm_enabled`: habilita service map por trafego observado.
- `workload_security_enabled`: adiciona `body.workload_security`.
- `network_passive_mode=ebpf`: solicita system probe; se o host nao suportar, degrada para socket/netlink.
- `network_pcap_enabled=true`: tenta PCAP via `tcpdump` quando disponivel.

## Contrato aditivo

O payload antigo permanece:

- `capture`;
- `flows`;
- `socket_snapshot`;
- `peers`;
- `listeners`;
- `interfaces`;
- `local_context`;
- `passive_advanced`.

Novos blocos opcionais:

- `service_map.services`: servicos confirmados por listener ou inferidos por processo/porta/trafego, com `env` e `version` quando vierem de metadado explicito ou cmdline reconhecido.
- `service_map.dependencies`: relacao `source_service -> target_service`, tipo do destino, porta, protocolo, amostras e trafego estimado.
- `service_map.system_probe`: modo pedido, modo aplicado, suporte eBPF, fallback e capacidades.
- `network_performance`: estados de conexao, top talkers e portas administrativas expostas.
- `workload_security`: sinais versionados para SOC, sempre sem acao destrutiva automatica.

## Privacidade

Os blocos novos nao exportam usuario, comando completo nem path absoluto. IP publico e mascarado para `/24`. Campos antigos seguem preservados por compatibilidade e devem ser tratados pelo backend conforme a politica de retencao atual de `network_capture`.

## Regras locais

`workload_security.rule_version=network-workload-v1`.

Sinais iniciais:

- porta administrativa exposta em escopo publico ou desconhecido;
- handshake falho recorrente;
- instabilidade DNS;
- IP publico sem contexto de reputacao;
- processo suspeito com destino publico.

Esses sinais sao evidencia para SOC/NOC. O agente nao bloqueia processo, arquivo ou conexao.

## Validacao local realizada

- `go test ./internal/platform/collectors/networkcapture`

Cobertura focada:

- servico legado sem OTEL aparece como `inferred`;
- `service`, `env` e `version` sao preenchidos por metadado explicito ou cmdline com evidencia `env:*`/`version:*`;
- dependencia para banco por DNS/porta vira `service -> database`;
- eBPF fica inativo com fallback quando nao aplicado;
- NPM mascara IP publico;
- workload security emite sinais sem acao destrutiva.
- web persiste `service_map.dependencies` e `workload_security.signals` como `AgenteNetworkRelation`, usando sinais como evidencia SOC/NOC sem bloqueio automatico.

Validacao web focada:

```bash
vendor/bin/codecept run -c api api ContractCest:ingestNetworkCaptureServiceMapEWorkloadSecurityPersistemRelacoes
```

Revalidacao em 2026-06-19 na branch `mayder/agente-datadog-paridade`:

- `go test ./internal/platform/collectors/networkcapture`
- `vendor/bin/codecept run -c api api ContractCest:ingestNetworkCaptureServiceMapEWorkloadSecurityPersistemRelacoes`
- `vendor/bin/codecept run -c common unit services/agent/AgentNetworkCaptureThresholdSignalServiceTest.php`
- `vendor/bin/codecept run -c common unit services/agent/AgentNetworkCaptureThresholdQueueServiceTest.php`

Todos passaram. Isso nao fecha homologacao real.

Fechamento controlado em 2026-06-19:

- `PKG70_EVIDENCE_DIR=/Users/brenomayder/projects/desktop/aiceberg_agent/docs/evidence/pkg70/network-usm-workload-20260619T192000Z go test ./internal/platform/collectors/networkcapture -run TestPKG70NetworkUSMWorkloadEvidence -count=1 -v`
- Bundle: `docs/evidence/pkg70/network-usm-workload-20260619T192000Z`.
- Cobertura: fluxo `web -> api -> postgres`, dependencia `service -> service` e `service -> database`, fallback quando eBPF nao esta suportado, contrato de eBPF ativo apenas quando `applied_mode` contem `ebpf_probe`, porta administrativa publica com `SYN_SENT`, sinais SOC sem acao destrutiva, `source_score` e redaction de IP publico nos blocos NPM/workload.
- O JSON bruto da evidencia preserva somente outputs redigidos e contadores controlados; flows originais de entrada nao sao anexados.

## Homologacao real complementar

- Permissao sem eBPF, Docker, Kubernetes e overhead operacional ficam cobertos pelos bundles reais do PKG-69: `permission-ebpf`, `docker-runtime`, `kubernetes-rbac` e `high-volume-overhead`.
- eBPF kernel ativo real nao e declarado como probe produtivo obrigatorio nem como superioridade. A versao atual valida o contrato, o fallback e o estado `ebpf_probe` apenas quando suportado/aplicado.
- SOC/NOC real deve continuar monitorando os sinais como evidencia, sem bloqueio automatico.

## Rollback

Desligar configs remotas:

- `network_advanced_enabled=false`;
- `usm_enabled=false`;
- `workload_security_enabled=false`;
- `network_pcap_enabled=false`;
- `network_passive_mode=socket`.
