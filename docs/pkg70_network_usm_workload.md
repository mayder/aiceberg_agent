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

## Pendencias de homologacao real

- kernel Linux com eBPF habilitado;
- Linux sem permissao/eBPF para validar fallback operacional;
- fluxo controlado `web -> api -> db`;
- container e Kubernetes reais correlacionando labels/pods com trafego;
- overhead de CPU/memoria com `network_advanced_enabled`, `usm_enabled` e `workload_security_enabled`;
- SOC/NOC consumindo os sinais em ambiente real.

## Rollback

Desligar configs remotas:

- `network_advanced_enabled=false`;
- `usm_enabled=false`;
- `workload_security_enabled=false`;
- `network_pcap_enabled=false`;
- `network_passive_mode=socket`.
