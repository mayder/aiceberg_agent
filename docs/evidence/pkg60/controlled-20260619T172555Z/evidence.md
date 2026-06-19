# PKG-60 - Evidencia Controlada de Pipeline de Logs

## Ambiente

- Data UTC: 2026-06-19T17:25:55Z
- Responsavel: Codex
- Cliente/lab: lab local controlado
- Ambiente: Darwin 25.5.0 arm64
- Versao agente: workspace atual
- Topologia: direct -> AIceberg para contrato de logs; Relay/Hub nao envolvidos neste teste controlado.

## Evidencia coberta

- Graylog/GELF: coberto por teste controlado de parser/classificacao.
- Linux auth: coberto por teste controlado de arquivo auth.log.
- App JSON: coberto por teste controlado com severity, service e redaction.
- Log texto comum: coberto por teste controlado de arquivo simples.
- Cursor/restart/truncamento/rotacao: coberto por teste controlado do coletor POSIX.
- Windows Security: coberto por parser no build Windows.
- Sysmon: coberto por parser no build Windows.
- Windows build: compilacao cruzada do pacote oslogs concluida.

## Comandos

- `go test ./internal/platform/collectors/oslogs`
- `go test ./internal/platform/collectors/oslogs ./internal/common/config ./internal/domain/usecase ./internal/bootstrap`
- `GOOS=windows GOARCH=amd64 go test -c -o raw/aiceberg_oslogs_windows.test.exe ./internal/platform/collectors/oslogs`

## Resultado

- Status: pass
- Evidencia bruta anexada: raw/pkg60-controlled-raw.tgz
- SHA256 bruto: 1926b4301cadc6d8f138cbf85127fe0867f736dc3da9baa0190d4ca9cba074a3
- Tamanho bruto bytes: 948
- SHA256 binario Windows de teste: 2a757b1a2dfe6582b4af8c12ceece39b092c57ff483c1a9f7a9afda6215c1506
- Tamanho binario Windows de teste bytes: 5688832
- Binario Windows retido no repo: no

## Limites

Esta evidencia e controlada. Ela reduz risco funcional, mas nao fecha as validacoes reais ainda abertas de PKG-60: Graylog real, Windows Security real, Linux auth real e Sysmon real em ambiente operacional.
