# 🧩 Arquitetura do AIceberg Agent

Este documento descreve a **arquitetura técnica do AIceberg Agent**, seu design modular, camadas, responsabilidades, e o fluxo de comunicação com o backend do **AIceberg**.

O agente foi projetado com base em princípios **SOLID**, **Clean Architecture** e **Ports & Adapters**, permitindo fácil evolução, testabilidade e portabilidade entre sistemas operacionais (Linux e Windows).

---

## 🧱 Visão Geral

O **AIceberg Agent** é um serviço residente (daemon / Windows Service) responsável por:

1. **Coletar** dados locais de telemetria (NOC) e segurança (SOC).
2. **Armazenar** temporariamente os dados (modo offline) em uma fila local persistente.
3. **Enviar** pacotes batelados (JSON comprimido) ao backend via **HTTP + JSON**.
4. **Manter** um canal de comunicação seguro e resiliente com o servidor AIceberg.

Na primeira versão (v0.1), o agente é **somente emissor** (envia dados).  
Versões futuras incluirão recepção de políticas, execução de comandos e atualizações remotas.

---

## ⚙️ Camadas da Arquitetura

```markdown
+----------------------------------------------------+
| cmd/agent |
| (composition root / entrypoint principal) |
+----------------------------------------------------+
| app |
| Orquestra o ciclo de vida e dependências |
+----------------------------------------------------+
| internal/common |
| - config: carrega YAML |
| - logger: abstração de log |
| - health: endpoint local opcional |
+----------------------------------------------------+
| ports (interfaces) |
| Definem contratos: Collector, Queue, Transport |
+----------------------------------------------------+
| implementations (adapters) |
| - collectors/noc/sysmetrics: coleta de sistema |
| - queue/bolt: fila local persistente |
| - ingest/httpjson: envio HTTP + JSON gzip/zstd |
| - ingest/batcher: agrega e envia em lotes |
+----------------------------------------------------+
| pkg (entidades) |
| - types/envelope: DTOs de telemetria |
| - version: versão do agente |
+----------------------------------------------------+
```

---

## 🧩 Fluxo de Execução

1. **Início do serviço**

   - O agente é iniciado automaticamente (systemd/Windows Service).
   - Lê a configuração `config.yml` e inicia o logger.

2. **Coleta**

   - Módulo `sysmetrics` coleta CPU, memória, disco e rede.
   - Gera um **envelope JSON** (`pkg/types/envelope.go`).

3. **Fila local**

   - Envelopes são armazenados no `queue/bolt` (usando bbolt).
   - Garante persistência offline e limites de uso de disco.

4. **Batching**

   - O módulo `batcher` agrega eventos até atingir:
     - X bytes (ex.: 256 KB), ou
     - Y segundos (ex.: 2s).
   - Serializa tudo em um array JSON e comprime (gzip/zstd).

5. **Envio**

   - O módulo `httpjson` envia `POST /v1/ingest` com:
     - Header `Authorization: Bearer <token>`,
     - `Content-Encoding: gzip`,
     - `Content-Type: application/json`.

6. **ACK**

   - O backend retorna `{stored, failed, duplicates}`.
   - O agente confirma (commit) e remove do bbolt.

7. **Retentativa**
   - Falhas (`429`, `5xx`, timeout) → backoff exponencial + retry.
   - Após reconexão (internet restaurada), retoma envio automático.

---

## 🔒 Segurança

- Comunicação **HTTPS (TLS 1.3)** com **autenticação via token** (ou mTLS futura).
- Arquivo de configuração local com permissões restritas.
- Processo roda com **usuário dedicado** (`aiceberg_agent`).
- Dados sensíveis (tokens, IDs) nunca são logados.
- Binário e políticas assinados digitalmente (planejado).

---

## 🧰 Principais Tecnologias

| Área         | Tecnologia                    | Finalidade                               |
| ------------ | ----------------------------- | ---------------------------------------- |
| Linguagem    | **Go (Golang)**               | desempenho, binário único, cross-compile |
| Configuração | **YAML (gopkg.in/yaml.v3)**   | configuração legível e segura            |
| Fila local   | **bbolt**                     | persistência local leve                  |
| Envio        | **net/http + gzip/zstd**      | comunicação resiliente e compacta        |
| Coleta       | **gopsutil/v3**               | métricas de sistema multiplataforma      |
| Logging      | **std/zap** (abstraído)       | logs estruturados                        |
| Sistema      | **systemd / Windows Service** | inicialização automática                 |

---

## 📡 Comunicação com o Backend (HTTP+JSON)

```markdown
[Agent] --POST /v1/ingest--> [API AIceberg]
|-- compress(gzip/zstd)
|-- retry/backoff/ACK
```

**Envelope JSON (simplificado):**

```json
{
  "envelope_id": "01JV...",
  "agent_id": "host-123",
  "kind": "metric",
  "sub": "sys",
  "ts_unix_ms": 1730123456789,
  "meta": { "os": "linux", "host": "web-01" },
  "body": { "cpu_pct": 7.1, "mem_pct": 43.3, "disk_pct": 48.0 }
}
```

---

## 🌐 Implantação

### Linux

- Instalado em `/usr/local/bin/aiceberg_agent`
- Configuração em `/etc/aiceberg/config.yml`
- Serviço systemd:

  ```bash
  sudo systemctl enable --now aiceberg_agent
  sudo systemctl status aiceberg_agent
  ```

### Windows

- Instalado em `C:\Program Files\AIceberg\agent\agent.exe`
- Configuração em `C:\ProgramData\AIceberg\config.yml`
- Serviço:

  ```powershell
  sc.exe create AIcebergAgent binPath= "C:\Program Files\AIceberg\agent\agent.exe -config C:\ProgramData\AIceberg\config.yml" start= auto
  sc.exe start AIcebergAgent
  ```

---

## 🔄 Evolução Planejada

| Versão   | Principais recursos                                       |
| -------- | --------------------------------------------------------- |
| **v0.1** | Envio HTTP/JSON (métricas básicas, fila offline)          |
| **v0.2** | Heartbeat, compressão zstd, retry/backoff refinado        |
| **v0.3** | Canal de controle (long-poll), ACK remoto                 |
| **v0.4** | Atualização remota, assinatura de binários                |
| **v1.0** | Transição opcional para gRPC + Protobuf com stream duplex |

---

## 🧭 Referências

- [Estrutura Inicial](Estrutura_Inicial.md)
- [Requisitos NOC](Requisitos_NOC.md)
- [Requisitos SOC](Requisitos_SOC.md)
- [Dicionário do projeto](Glossario.md)
- [Base pro futuro](Base.md)

---

## 📄 Licença

Projeto interno do **AIceberg** — todos os direitos reservados.  
Desenvolvido sob orientação do **Arquiteto do Caos Elegante**.

---

[⬅️ Voltar ao README](../../README.md)
