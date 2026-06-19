# PKG-69 - Relay Hub Direct Hosts

## Ambiente

- Data UTC: 2026-06-19T03:57:49Z
- Responsavel: Codex
- Cliente/lab: Docker network dedicada com containers separados
- Ambiente: Hosts separados
- Host/agente/HUB/relay: direct, hub, relay e backend em containers separados
- Versao agente: 0.8.8
- Artefato instalado: binario Linux arm64 construido do workspace atual; hashes em BUILD_SHA256SUMS
- Topologia: direct/hub/relay hosts separados

## Evidencia obrigatoria

- direct_host_id: docker:3238ff45487c.24.0.5
- hub_host_id: docker:d711556feee6.24.0.3
- relay_host_id: docker:b13360616801.24.0.4
- relay_upstream_host_id: docker:d711556feee6.24.0.3
- direct -> AIceberg confirmado: yes
- hub -> AIceberg confirmado: yes
- relay -> hub -> AIceberg confirmado: yes
- relay sem conexao direta com API AIceberg: yes
- agentless via Hub quando aplicavel: yes

## Metricas

- direct_ingested: yes
- hub_ingested: yes
- relay_ingested_via_hub: yes
- relay_direct_api_attempts: 0

## Resultado

- Status: pass
- Evidencia bruta anexada: raw/aiceberg_pkg69_relay_hub_direct_raw_20260619T035635Z.tgz
- Observacoes: backend registrou token redigido do Relay vindo do IP do Hub, nao do IP do Relay; agentless jobs e observations passaram pelo fluxo controlado; arquivos de estado com tokens de lab foram removidos do artefato
- Rollback validado: yes
- Revisor: Codex
- Aprovacao fechamento: yes
