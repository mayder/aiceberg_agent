# Evidencia PKG-74 - Discovery controlado

- Data UTC: 2026-06-20T18:00:00Z
- Cenario: descoberta de fontes para aplicacao lenta com web/app/banco/rede, runtime e seguranca
- Ambiente: teste controlado Go darwin/arm64
- Host controlado: pkg74-controlled-host
- Candidatos descobertos: 24
- Lacunas registradas: 1
- Produtos cobertos: apache, application, containerd, docker, iis, kubernetes, linux_auth, mysql, nginx, node_app, opentelemetry, postgresql, python_app, web_server
- Politica: bounded=true, read_only=true, min_severity=error, activates_collection=false
- Redaction: token/senha sensivel ausente do payload bruto
- Rede: portas 8080 e 5432 classificadas como web_server e postgresql por contrato
- Evidencia bruta anexada: raw/pkg74-discovery-controlled-raw.tgz

Esta evidencia cobre o cenario controlado de troubleshooting de aplicacao lenta sem declarar causa raiz. IIS e Kubernetes reais continuam dependentes de ambiente com esses componentes presentes.
