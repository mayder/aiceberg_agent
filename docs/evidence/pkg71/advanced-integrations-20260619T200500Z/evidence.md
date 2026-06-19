# Evidencia PKG-71 - Integracoes avancadas

- Data UTC: 2026-06-19T20:05:00Z
- Cenario: OpenMetrics, JMX/Jolokia, WMI/IIS guard, PostgreSQL, MySQL falha controlada, RabbitMQ, Nginx e bloqueio beta sem homologacao
- Ambiente: teste controlado Go darwin/arm64
- OpenMetrics /metrics: ok
- JMX Jolokia: ok
- Windows WMI/IIS: skipped
- PostgreSQL reachability: ok
- MySQL falha controlada: critical
- RabbitMQ reachability: ok
- Nginx HTTP: ok
- Beta sem homologacao: blocked
- credentials_ref_leaked: no
- denied_metric_leaked: no
- Evidencia bruta anexada: raw/pkg71-advanced-integrations-raw.tgz

Esta evidencia fecha validacao controlada do PKG-71. Windows Server real, bancos produtivos e marketplace amplo continuam exigindo homologacao por cliente antes de ativacao ampla; beta/experimental seguem bloqueados sem homologation_ref.
