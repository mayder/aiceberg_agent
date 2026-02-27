# Pacote enviado para `POST /api/v1/ingest`

O agente envia um *array* de envelopes JSON (batch). Cada item segue o contrato abaixo.

## Status de entrega (PKG-19 observabilidade/remediação)

- [x] Cliente remoto para `GET /v1/agent/selfheal-commands`.
- [x] Cliente remoto para `POST /v1/agent/selfheal-report`.
- [x] Cliente remoto para `POST /v1/agent/error-report`.
- [x] Execução local de comandos de auto-remediação seguros (`restart_agentless_worker`, `reload_configuration`, `clear_local_lock`, `requeue_pending_collect`, `validate_api_connectivity`, `resync_clock`).
- [x] Loop principal com polling contínuo de self-healing (`SELFHEAL_POLL_INTERVAL`).
- [x] Telemetria de erros operacionais do worker/coleta/flush para painel web de saúde.
- [x] Documentação operacional atualizada no `README.md` (seção de self-healing e error-report).
- [x] Worker agentless em modo `hub` inicializa independente do env `AGENTLESS_ENABLED`, com enable/disable efetivo controlado por prefs remotas após primeiro config sync.
- [x] Novo comando remoto `inspect_runtime_config` para retornar snapshot sanitizado de runtime (mode, flags env/prefs, estado efetivo e disponibilidade do worker).

## Estrutura do envelope

```jsonc
{
  "envelope_id": "20251215T162200.868991877",
  "agent_id": "host-01",
  "schema_version": 1,
  "kind": "metric",
  "sub": "sysmetrics",
  "ts_unix_ms": 1765815720868,
  "body": { /* ver seções abaixo */ }
}
```

## Campos do `body`

- `capabilities`: mapa bool indicando o que foi coletado (cpu, memory, disk, network, net_active, host, sensors, power, sanity, gpu, services, time_sync, vulns, inventory, logs, updates, agent, processes).
- `cpu`: percentuais, loads, contagem de cores, frequência.
- `memory`: total/used/free/buffers/cached + swap.
- `disk`: `filesystems[]` (mount, fs_type, bytes, inodes), `io_stats[]` (reads/writes/bytes/times), `smart[]` (health/temp por device).
- `network`: `interfaces[]` (bytes/packets/errors, flags, ips, mac, mtu, is_up).
- `net_active`: `connections_by_state`, `listening[]` (proto, local_addr, local_port).
- `host`: hostname, os, platform, platform_family/version, kernel_version, uptime_sec, boot_time_unix, virtualização.
- `sensors`: temperaturas/fans (quando disponíveis).
- `power`: baterias (percentual, estado, capacidade).
- `sanity`: `ping[]` e `dns[]` (target, success, duration_ms, error).
- `gpu`: lista (vendor, name, memória, util, temp, fan, power).
- `services`: nome/status/enabled.
- `time_sync`: source, offset_ms, rtt_ms, error, last_check_unix.
- `vulns`: `cves[]` (resultados da detecção local).
- `logs`: arquivos `.log` locais (path, size_bytes).
- `updates`: veja seção detalhada abaixo.
- `agent`: queue_items, queue_bytes, version.
- `processes`: top processos (pid, name, cpu_percent, rss_bytes, vms_bytes, io_read_bytes/io_write_bytes, create_time_unix, status, cmdline).
- `inventory`: veja seção detalhada abaixo.

## Inventory (para CVE)

### Linux

- `linux_rpm_packages[]`: `{name, epoch, version, release, arch, vendor, source}` usando `rpm -qa --qf '%{NAME}\t%{EPOCHNUM}\t%{VERSION}\t%{RELEASE}\t%{ARCH}\t%{VENDOR}\n'` (ou dpkg simplificado quando for Debian-like).
- `os_release`: `{id, version_id, pretty_name}` de `/etc/os-release`.
- `kernel`: `{running, installed[]}` sendo `running` o `uname -r` e `installed[]` pacotes kernel* com EVR.
- `repos`: `{enabled[], raw}` do `dnf|yum repolist -v`.

### Windows

- `windows_hotfixes[]`: `{id, installed_on, description, source}` de `Get-HotFix`.
- `windows_apps[]`: `{name, version, vendor, install_date, source, install_location, install_source, uninstall_string}` das chaves de Uninstall 32/64 bits.
- `windows_features[]`: `{name, display_name, installed}` de `Get-WindowsFeature`.

## Updates (patch level)

- Array `updates[]` contendo objetos:
  - `source`: ex. `dnf`, `apt`, `windows_update`, `softwareupdate`.
  - `pending`: contagem (fallback).
  - `last_check_unix`: epoch segundos.
  - `error`: mensagem opcional.
  - `security`: detalhamento:
    - `advisories[]`: `{advisory_id, severity, cves[], packages[]}` (Linux via `dnf|yum updateinfo --security info`).
    - `pending_updates[]`: `{update_id, title, kb_ids[], category, severity, is_downloaded, is_installed}` (Windows Update).
    - `pending_count`: contagem agregada de pendências de segurança.

## Exemplo resumido de `body` (Linux RHEL)

```jsonc
{
  "capabilities": {"cpu":true,"memory":true,"disk":true,"network":true,"net_active":true,"host":true,"inventory":true,"updates":true},
  "cpu":{"percent_total":37.1,"load1":2.8,"cores_logical":4},
  "memory":{"total_bytes":12265316352,"used_bytes":7857442816,"swap_used_bytes":4225724416,"swap_total_bytes":4227854336,"swap_used_percent":99.95},
  "disk":{"filesystems":[{"mount":"/","fs_type":"xfs","total_bytes":60752080896,"used_bytes":30473637888,"used_percent":50.16}],"io_stats":[{"device":"sda","reads":25495131,"writes":71034875,"read_bytes":1103123730432,"write_bytes":13130897314816}],"smart":[{"device":"/dev/sda1","health":"OK"}]},
  "network":{"interfaces":[{"name":"ens192","bytes_recv":4011189785584,"bytes_sent":132389766947,"ips":["10.240.0.19/22"],"is_up":true}]} ,
  "net_active":{"connections_by_state":{"ESTABLISHED":82,"LISTEN":12},"listening":[{"proto":"tcp","local_addr":"0.0.0.0","local_port":27017}]} ,
  "host":{"hostname":"rhl-ctg-hml-02.sadatrans.local","platform":"redhat","platform_version":"8.10","kernel_version":"4.18.0-553.33.1.el8_10.x86_64","uptime_sec":2437521},
  "inventory":{
    "linux_rpm_packages":[{"name":"openssl","epoch":0,"version":"1.1.1k","release":"14.el8_6","arch":"x86_64","vendor":"Red Hat, Inc.","source":"rpm"}],
    "os_release":{"id":"rhel","version_id":"8.10","pretty_name":"Red Hat Enterprise Linux 8.10"},
    "kernel":{"running":"4.18.0-553.33.1.el8_10.x86_64","installed":[{"name":"kernel","epoch":0,"version":"4.18.0","release":"553.33.1.el8_10","arch":"x86_64","vendor":"Red Hat, Inc.","source":"rpm"}]},
    "repos":{"enabled":["rhel-8-baseos"],"raw":"...repolist -v output..."}
  },
  "updates":[{
    "source":"dnf",
    "last_check_unix":1765815718,
    "security":{
      "advisories":[{"advisory_id":"RHSA-2025:xxxx","severity":"Important","cves":["CVE-2025-xxxx"],"packages":["kernel.x86_64 4.18.0-553.33.1.el8_10"]}],
      "pending_count":1
    },
    "pending":1
  }]
}
```

## Exemplo resumido de `body` (Windows)

```jsonc
{
  "capabilities":{"inventory":true,"updates":true,"net_active":true,"services":true},
  "inventory":{
    "windows_hotfixes":[{"id":"KB5031234","installed_on":"2025-01-10","description":"Security Update","source":"NT AUTHORITY\\SYSTEM"}],
    "windows_apps":[{"name":"Google Chrome","version":"120.0.0","vendor":"Google","install_date":"20250105","source":"registry","install_location":"C:\\\\Program Files\\\\Google\\\\Chrome"}],
    "windows_features":[{"name":"Web-Server","display_name":"Web Server (IIS)","installed":true}]
  },
  "updates":[{
    "source":"windows_update",
    "last_check_unix":1765815718,
    "security":{
      "pending_updates":[{"title":"2025-01 Cumulative Update","kb_ids":["KB5035678"],"category":"Security Update","severity":"Critical","is_downloaded":false,"is_installed":false}],
      "pending_count":1
    },
    "pending":1
  }],
  "services":[{"name":"MSSQLSERVER","status":"running"}],
  "net_active":{"listening":[{"proto":"tcp","local_addr":"0.0.0.0","local_port":3389}]}
}
```
