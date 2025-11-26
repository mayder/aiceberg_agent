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

**Envelope JSON (versão atual do `sysmetrics`):**

```json
{
  "envelope_id": "20251124T124447.485921000",
  "agent_id": "host-01",
  "kind": "metric",
  "sub": "sysmetrics",
  "schema_version": 1,
  "ts_unix_ms": 1763988287485,
  "body": {
    "cpu": { "percent_total": 4.2, "percent_per_cpu": [3.1, 5.3], "load1": 0.12, "cores_logical": 8, "cores_physical": 4, "freq_current_mhz": 2400 },
    "memory": { "total_bytes": 17179869184, "used_bytes": 8590458880, "used_percent": 50.0, "swap_total_bytes": 2147483648, "swap_used_bytes": 0 },
    "disk": {
      "filesystems": [{ "mount": "/", "fs_type": "ext4", "total_bytes": 107374182400, "used_bytes": 53687091200, "used_percent": 50 }],
      "io_stats": [{ "device": "nvme0n1", "reads": 1234, "writes": 5678, "read_bytes": 1048576, "write_bytes": 2097152 }],
      "smart": [{ "device": "/dev/nvme0n1", "health": "PASSED", "temperature_c": 35 }]
    },
    "network": {
      "interfaces": [{ "name": "eth0", "mac": "00:11:22:33:44:55", "ips": ["192.0.2.10/24"], "bytes_sent": 123456, "bytes_recv": 654321, "is_up": true }]
    },
    "net_active": {
      "connections_by_state": { "ESTABLISHED": 12, "LISTEN": 5 },
      "listening": [{ "proto": "tcp", "local_addr": "0.0.0.0", "local_port": 8080 }]
    },
    "host": { "hostname": "host-01", "os": "linux", "platform": "ubuntu", "kernel_version": "6.8.0", "uptime_sec": 123456, "boot_time_unix": 1763900000, "virtualization": "kvm" },
    "sensors": { "temperatures": [{ "sensor": "CPU", "temp_c": 42.5 }], "fans": [{ "sensor": "fan1_input", "rpm": 1800 }] },
    "power": { "batteries": [{ "percent": 78.1, "state": "Discharging", "design_capacity_wh": 50.0, "full_capacity_wh": 48.0, "charge_rate_mw": 12000, "voltage_v": 11.4 }] },
    "gpu": [{ "vendor": "nvidia", "name": "RTX 3080", "memory_total_mb": 10240, "memory_used_mb": 2048, "util_percent": 15.0, "temperature_c": 50, "fan_percent": 30, "power_w": 80 }],
    "services": [{ "name": "ssh.service", "status": "running" }],
    "time_sync": { "source": "time.google.com", "offset_ms": 2, "rtt_ms": 24, "last_check_unix": 1763988287 },
    "sanity": {
      "ping": [{ "target": "1.1.1.1:53", "success": true, "duration_ms": 10 }],
      "dns": [{ "target": "example.com", "success": true, "duration_ms": 15 }]
    },
    "agent": { "queue_items": 3, "queue_bytes": 0 },
    "logs": [{ "path": "./logs/agent.log", "size_bytes": 12345 }],
    "updates": [{ "source": "apt", "pending": 5 }],
    "processes": [{ "pid": 1234, "name": "nginx", "cpu_percent": 2.1, "rss_bytes": 10485760 }]
  }
}
```

**Blocos coletados pelo `sysmetrics` (todos opcionais/conforme suporte do SO):**
- `cpu`, `memory`, `disk` (fs + I/O + SMART), `network` (interfaces), `net_active` (conexões e listens).
- `host` (OS/platform/kernel/uptime/virtualização).
- `sensors` (temperaturas, fans), `power` (bateria), `gpu` (via `nvidia-smi`).
- `services` (systemd/sc), `time_sync` (NTP), `sanity` (ping/DNS), `agent` (backlog da fila), `logs` (tamanho de .log em ./logs), `updates` (apt/softwareupdate), `processes` (top 5 CPU).

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
