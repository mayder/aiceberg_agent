# Evidencia PKG-70 - Network USM workload

- Data UTC: 2026-06-19T19:20:00Z
- Cenario: fluxo controlado web -> api -> db, eBPF fallback/suporte declarado, porta administrativa exposta e workload security evidence-only
- Ambiente: teste controlado Go darwin/arm64
- Dependencia web -> api: validada por service_map.dependencies
- Dependencia api -> postgres: validada como target_type=database
- eBPF sem suporte/permissao: fallback socket_snapshot/netlink sem marcar ebpf_active
- eBPF suportado: contrato marca ebpf_active apenas quando applied_mode contem ebpf_probe
- Degradacao/porta exposta: SYN_SENT em porta 22 publica gera exposed_admin_ports e sinais SOC
- Overhead/redaction: source_score calculado, IP publico mascarado em NPM e workload_security
- Acoes destrutivas: false
- Evidencia bruta anexada: raw/pkg70-network-usm-workload-raw.tgz

Esta evidencia fecha a validacao controlada do PKG-70 no agente. eBPF kernel ativo real nao e declarado como superioridade nem como probe produtivo obrigatorio; a operacao permanece opt-in com fallback seguro e os bundles reais do PKG-69 complementam Docker, Kubernetes, permissao e overhead.
