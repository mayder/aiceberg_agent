# PKG-69 - Windows desktop real com agente do cliente

- Data UTC: 2026-06-19T16:42:39Z
- Cenario: Windows desktop
- Host: GOVI, Windows 10 Home Single Language, 192.168.15.24
- Agente AIceberg: id 72, cliente 1, ativo 153
- Versao validada: 0.8.10
- Artefato Windows: `dist/aiceberg-agent-windows-amd64.zip`
- SHA-256 do ZIP: `8f510bf8541bb21d0b0f862b9a3509508c668f31ceb7ed84621acdb9e0ab368f`
- SHA-256 do binario Windows: `708c05958424d28883a39ace55a3dd289416843b0b96bfc6dbe2512235241251`

## Ajustes validados

- WinRM estava acessivel em `5985` para operacao remota controlada.
- O servico `AIcebergAgent` foi atualizado para `0.8.10` com backup local antes da troca.
- `OSLOG_ENABLED=true`, `OSLOG_MIN_SEVERITY=error`, canais `Application,System` e cursor persistente em `C:\ProgramData\AIceberg\data\oslogs.cursor`.
- O coletor Windows passou a ler `wevtutil` em XML para obter `EventRecordID` de forma estavel.
- O filtro de severidade minima foi aplicado no XPath (`Level<=2`) antes do limite de lote, evitando perder erro quando eventos informativos recentes ocupam o lote.
- Niveis localizados como `Erro` sao normalizados para `error`.

## Evidencia local

- Evento controlado criado no Windows:
  - Event ID: `6907`
  - Record ID: `8474`
  - LevelDisplayName: `Erro`
  - Mensagem: `AIceberg controlled Windows desktop ERROR final ingestion proof 2026-06-19T16:39:28Z`
- Health local apos coleta:
  - `status=ok`
  - `version=0.8.10`
  - `proc_cpu_percent=1.4046416530304082`
  - `proc_rss_bytes=36896768`
  - canal direct conectado em `https://api.aiceberg.com.br/v1/agent/channel`
- Cursor apos coleta:
  - `Application=8474`
  - `System=15734`

## Evidencia no backend

Consulta read-only no banco do AIceberg:

- `agente.id=72`
- `agente.versao_agente=0.8.10`
- `agente.ultimo_seen_em=2026-06-19 16:42:35`
- `ativo.id=153`
- `fonte_log.id=21`
- `fonte_log.status_coleta_id=4`
- `fonte_log.data_ultima_coleta=2026-06-19 16:40:19`
- `log_bruto.id=17163824`
- `log_bruto.log_nivel_id=3`
- `log_bruto.data_log=2026-06-19 13:39:28`
- `log_bruto.repeticoes=1`
- `log_bruto.conteudo=AIceberg controlled Windows desktop ERROR final ingestion proof 2026-06-19T16:39:28Z`

## Resultado

Validado em ambiente real que o agente Windows desktop coleta EventLog com severidade minima `error`, normaliza nivel localizado, envia level/severity inferidos e persiste o log no backend vinculado ao ativo/agente corretos.

## Pendencias

- O reboot real foi validado posteriormente no bundle `docs/evidence/pkg69/reboot-during-collection-20260619T165440Z`.
- O usuario temporario/WinRM usado para validacao deve ser removido/endurecido quando a janela operacional acabar.
