# PKG-69 - rollout remoto 0.8.9 via web para cliente 1

Data UTC: 2026-06-19T12:18:26Z

Escopo:
- Cliente: `1`.
- Agentes ativos atualizados: `1`, `4`, `18`, `19`, `70`, `71`.
- Versao origem predominante: `0.8.8`.
- Versao alvo: `0.8.9`.
- Artefatos oficiais publicados:
  - Linux amd64: `cliente/web/downloads/agent/0.8.9/aiceberg-agent-linux-amd64.tar.gz`.
  - Windows amd64: `cliente/web/downloads/agent/0.8.9/aiceberg-agent-windows-amd64.zip`.

Resultado final no backend:

| Agente | Host | OS/arch | Versao final | Status update | Reportado em |
| --- | --- | --- | --- | --- | --- |
| `1` | `dreamy-brown.92-42-106-121.plesk.page` | linux/amd64 | `0.8.9` | `version_confirmed` | `2026-06-19 12:13:32` |
| `4` | `VMAIPROD2` | linux/amd64 | `0.8.9` | `version_confirmed` | `2026-06-19 11:33:27` |
| `18` | `xenodochial-williams.92-204-189-13.plesk.page` | linux/amd64 | `0.8.9` | `version_confirmed` | `2026-06-19 12:13:27` |
| `19` | `zen-davinci.92-204-168-1.plesk.page` | linux/amd64 | `0.8.9` | `version_confirmed` | `2026-06-19 12:13:37` |
| `70` | `srv-ainalise-prod` | linux/amd64 | `0.8.9` | `version_confirmed` | `2026-06-19 12:15:13` |
| `71` | `WIN-LQTG0Q3JU26` | windows/amd64 | `0.8.9` | `version_confirmed` | `2026-06-19 12:18:26` |

Observacoes:
- Linux root/systemd (`1`, `18`, `19`) atualizou via comando remoto em background, validando SHA256 e reiniciando `aiceberg-agent`.
- Linux usuario/crontab (`70`) aplicou o pacote pelo fluxo remoto, mas o primeiro comando de restart apontava para `agent.pid` fora do caminho real; o binario foi aplicado e o processo foi reiniciado manualmente no caminho correto `/home/deploy/aiceberg-agent/data/agent.pid`. O comando remoto salvo foi corrigido para updates futuros.
- Windows (`71`) falhou primeiro porque `C:\Program Files\AIceberg\agent\aiceberg-agent-update-launcher.ps1` nao existia. O update foi reenfileirado com launcher em `D:\AIcebergAgentInstall` e confirmou `version_confirmed`.
- Todos os agentes continuaram reportando `ultimo_seen_em` apos o update.

Limitacoes:
- Esta evidencia valida rollout remoto real via backend web para os agentes ativos do cliente, incluindo Windows.
- Nao fecha PKG-69 sozinho: continuam pendentes gates globais como Windows desktop, reboot real em janela autorizada e aceite final do gate de evidencias.
