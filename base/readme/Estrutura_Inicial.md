# 📦 Estrutura Inicial do Projeto (AIceberg Agent)

Abaixo está a árvore de pastas e arquivos do agente, já organizada para seguir **SOLID / Ports & Adapters**, facilitando evolução e testes.

## 🌲 Árvore de diretórios

```text
aiceberg_agent/
├─ cmd/agent/                # entrypoint (composition root)
│  └─ main.go
├─ configs/
│  └─ config.example.yml     # config base (YAML)
├─ internal/
│  ├─ app/                   # orquestra o ciclo de vida (start/stop)
│  │  └─ app.go
│  ├─ common/                # utilidades transversais
│  │  ├─ config/config.go    # leitura/validação de config
│  │  ├─ logger/logger.go    # logging estruturado
│  │  └─ health/health.go    # /health local (opcional)
│  ├─ ports/                 # interfaces (DIP - Dependency Inversion)
│  │  ├─ collector.go        # ISP: contrato de coletores
│  │  ├─ encoder.go          # encoder (json)
│  │  ├─ queue.go            # persistência local (bbolt/…)
│  │  └─ transport.go        # transporte (HTTP/JSON)
│  ├─ collectors/            # implementações concretas (NOC/SOC)
│  │  └─ noc/
│  │     └─ sysmetrics/sysmetrics.go
│  ├─ ingest/                # envio para backend
│  │  ├─ batcher/batcher.go  # agrega em lotes (tamanho/tempo)
│  │  └─ httpjson/client.go  # transporte HTTP (POST /v1/ingest, gzip)
│  └─ queue/
│     └─ bolt/queue.go       # implementação bbolt (store-and-forward)
├─ pkg/
│  ├─ types/envelope.go      # DTOs dos envelopes
│  └─ version/version.go     # versão do agente
├─ scripts/
│  ├─ linux/aiceberg_agent.service    # unit systemd (deploy)
│  └─ windows/install-service.ps1     # registro como Windows Service
├─ Makefile
├─ go.mod
└─ README.md
```

## 🧪 Prompt para continuação (implementar envio HTTP+JSON)

Use este prompt quando for me pedir o próximo passo (pode colar direto):

```text
Vamos continuar o desenvolvimento do AIceberg Agent (HTTP+JSON, envio apenas) na estrutura que criamos.
Objetivo do passo: implementar o caminho completo de envio
(coletor sysmetrics ➜ encode JSON ➜ queue/bolt ➜ ingest/batcher ➜ ingest/httpjson ➜ POST /v1/ingest),
com compressão gzip, idempotência por envelope_id e retry/backoff em 429/5xx.

Por favor:
1) entregue os códigos completos de:
   - internal/collectors/noc/sysmetrics/sysmetrics.go (gopsutil)
   - internal/queue/bolt/queue.go (bbolt)
   - internal/ingest/batcher/batcher.go (tamanho/tempo)
   - internal/ingest/httpjson/client.go (keep-alive + gzip + headers)
   - ajuste de internal/app/app.go para ligar tudo com goroutines e shutdown

2) adicione dependências no go.mod e comandos de teste (go run …)

3) exponha logs mínimos para acompanhar batch, envio e ACK

4) inclua testes simples (ou exemplos) para simular offline/online (429/5xx)
```

## ▶️ Como testar rapidamente

```bash
go mod init github.com/you/aiceberg_agent
go mod tidy
go run ./cmd/agent
```

> Dica: se o preview do Markdown “desformatar” sua árvore ou comandos, verifique se o bloco está entre **três crases** (```), sem indentação extra ou caracteres especiais fora do bloco.

---

[⬅️ Voltar ao README](../../README.md)
