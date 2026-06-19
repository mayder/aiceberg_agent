# PKG-69 - Proxy TLS

## Ambiente

- Data UTC: 2026-06-19T03:39:45Z
- Responsavel: Codex AIceberg
- Cliente/lab: lab local controlado de proxy/TLS
- Ambiente: Darwin 25.5.0 arm64, httptest proxy autenticado local e TLS controlado
- Host/agente/HUB/relay: client HTTP do agente em processos separados para evitar cache de proxy do runtime Go
- Versao agente: 0.8.8 / internal/common/httpx do commit HEAD
- Artefato instalado: probe temporario removido apos execucao; nenhum agente instalado alterado
- Topologia: direct -> AIceberg

## Evidencia obrigatoria

- proxy autenticado real: yes, proxy HTTP local exigiu Proxy-Authorization e recebeu 1 request
- TLS invalido rejeitado: yes, httptest TLS self-signed foi rejeitado pelo client do agente
- TLS valido aceito: yes, request HTTPS publico com certificado valido retornou HTTP 200
- sem token em log: yes, raw redige proxy env e scan local nao encontrou segredo conhecido
- rollback de config validado: yes, probes encerrados e arquivo temporario removido sem alterar config persistente

## Metricas

- requests_ok: 2
- requests_failed_expected: 1
- retry_count: 0

## Resultado

- Status: pass
- Evidencia bruta anexada: raw/aiceberg_pkg69_proxy_tls_raw_20260619T033944Z.tgz
- Observacoes: Proxy e TLS foram executados em processos separados porque http.ProxyFromEnvironment cacheia variaveis de proxy no processo Go. A prova cobre proxy autenticado local, TLS invalido rejeitado e TLS valido aceito pelo client HTTP do agente.
- Rollback validado: yes
- Revisor: Codex AIceberg
- Aprovacao fechamento: yes
