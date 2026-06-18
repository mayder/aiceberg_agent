# 🧠 AIceberg Agent

O **AIceberg Agent** é o componente responsável por coletar, consolidar e enviar dados de telemetria (NOC) e segurança (SOC) para o backend do **AIceberg**, operando de forma autônoma em **Linux e Windows**.  
Esta primeira versão implementa o envio via **HTTP + JSON**, com foco em simplicidade, resiliência e portabilidade.

---

## 📁 Estrutura de Documentação Base

Para manter o repositório mais limpo e organizado, os principais documentos de especificação e requisitos estão localizados na pasta:

```text
/base/readme/
```

Abaixo segue a descrição e o link de cada arquivo.

---

### 🧱 [Estrutura Inicial.md](/base/readme/Estrutura_Inicial.md)

Documento com a **árvore de diretórios**, responsabilidades de cada pasta/arquivo e o **prompt recomendado** para continuar o desenvolvimento (implementação do pipeline de envio HTTP+JSON). Use-o como guia rápido para navegar no projeto e pedir os próximos passos.

---

### 🔷 [Arquitetura.md](/base/readme/Arquitetura.md)

**Status:** em construção  
Contém a **visão de arquitetura completa** do agente, descrevendo camadas, módulos, responsabilidades e o fluxo de comunicação entre agente, backend e hubs intermediários.  
Este arquivo servirá como referência principal para diagramas, decisões técnicas e padrões de implementação (SOLID, ports/adapters, injeção de dependências).

---

### 🧩 [Requisitos NOC.md](/base/readme/Requisitos_NOC.md)

Lista completa e detalhada de **todas as funcionalidades esperadas de um NOC moderno** — incluindo coleta de métricas, monitoramento de rede, análise de disponibilidade, notificações e dashboards de desempenho.  
Serve como guia de escopo para os módulos de telemetria e monitoramento do agente.

---

### 🛡️ [Requisitos SOC.md](/base/readme/Requisitos_SOC.md)

Relaciona as **funcionalidades esperadas de um SOC** (Security Operations Center) — coleta de logs, detecção de ameaças, resposta a incidentes, correlação de eventos e governança de segurança.  
Este documento é usado como base para o roadmap da parte de segurança do agente.

---

### 📘 [Dicionário do projeto.md](/base/readme/Glossario.md)

Dicionário técnico e conceitual com a **definição de todos os termos, tecnologias e siglas** utilizadas no projeto AIceberg Agent.  
Inclui explicações sobre protocolos, padrões, bibliotecas Go e componentes do ecossistema (Prometheus, journald, mTLS, bbolt etc.).

---

### 🚀 [Base pro futuro.md](/base/readme/Base.md)

Documento de visão e continuidade.  
Registra **decisões arquiteturais, planos de expansão, ideias e melhorias futuras** do agente, incluindo a transição para gRPC, novos coletores, e estratégias de atualização e controle remoto.

---

## ✅ Funcionalidades atuais

- Coleta de sysmetrics (CPU, memória, disco, rede, serviços, time sync, sensores/bateria quando disponíveis) com prefs remotas para pausar/coletar.
- Coleta de logs do SO (tail de arquivos em Linux/macOS; Event Logs no Windows via `wevtutil`) controlada por `OSLOG_ENABLED` e `OSLOG_FILES`/`OSLOG_WIN_CHANNELS`, com cursor persistido.
- Modos `direct` (API), `relay` (envia para hub) e `hub` (escuta `/v1/ingest` e repassa para a API), incluindo transporte HTTP JSON para métricas e logs.
- Bootstrap por token (`/v1/agent/bootstrap`), persistindo `agent.token`/`bootstrap.ok`, health opcional e ping/config-sync periódicos.
- Carregamento de config via arquivo/env (`-config` ou `AGENT_ENV_FILE`) suportando env-file/JSON/YAML simples, com sobrescrita por variáveis de ambiente.
- Transporte HTTP com header `Idempotency-Key`, opção de gzip (`HTTP_GZIP=true`) e TLS opcionalmente inseguro para testes (`TLS_INSECURE_SKIP_VERIFY=true`), respeitando `HTTPS_PROXY/NO_PROXY`.
- Coletor de logs com defaults por SO, modo diagnóstico (`OSLOG_DIAG=true`) e mensagens de permissão/arquivo para facilitar troubleshooting.
- Net active inclui contagem por estado e todas as portas TCP/UDP em escuta (sem limite artificial).
- Outbox persistente com fallback em memória (`OUTBOX_PATH`, `OUTBOX_MAX_MB`).
- Instaladores gerados por `scripts/build_installers.sh` (tar.gz/zip) com service scripts para systemd, launchd e serviço Windows nativo.
- Observabilidade operacional do worker: envio de erros internos para `/v1/agent/error-report` (falhas de coleta, flush, sync e worker agentless).
- Auto-remediação remota: polling de comandos em `/v1/agent/selfheal-commands` e reporte de execução em `/v1/agent/selfheal-report` com trilha (`acked`, `running`, `success`, `failed`), incluindo `inspect_runtime_config` para snapshot sanitizado de runtime.

---

## 🧭 Próximos Passos

1. Ampliar cobertura de testes de integração para fluxo de self-healing (polling + report + fallback) e inspeção de runtime.  
2. Atualizar o documento [Arquitetura.md](/base/readme/Arquitetura.md) com diagrama e decisões atuais.  

---

## ⚙️ Execução Rápida

```bash
make tidy
make run
```

O agente carregará o arquivo de configuração `./configs/config.example.yml`, exibirá logs básicos e iniciará o ciclo de coleta e envio (modo esqueleto).

---

## 🧪 Teste E2E local (sem servidores)

O script abaixo sobe um backend fake e roda **direct**, **hub** e **relay** localmente, validando ingestão, ping/bootstraps e métricas:

```bash
scripts/e2e.sh
```

Opções úteis:

1. Manter os logs e diretórios temporários:
   ```bash
   E2E_KEEP=1 scripts/e2e.sh
   ```
2. Fixar portas (caso precise):
   ```bash
   E2E_BACKEND_PORT=8082 E2E_HUB_PORT=9090 scripts/e2e.sh
   ```

O script cria um workdir temporário com logs de cada instância e um backend fake em Go.
Se precisar inspecionar falhas, rode com `E2E_KEEP=1` e verifique os arquivos em `E2E_WORKDIR`.

---

## 🔎 Smoke test local (Linux/macOS e Windows)

Smoke test rápido para validar health, metrics e pipeline de oslogs em um único host:

```bash
scripts/smoke.sh
```

No Windows:

```powershell
.\scripts\smoke.ps1
```

## 🧪 Lab de Simulação Docker (multi-nós)

Para facilitar os testes de rede/Hub/Relay sem depender de infra externa, use o wrapper:

```bash
scripts/lab-simulacao.sh up
scripts/lab-simulacao.sh ps
```

Comandos principais:

```bash
# Diagnóstico do ambiente + status
scripts/lab-simulacao.sh doctor

# Copiar artefato linux-amd64 do dist para o lab
scripts/lab-simulacao.sh copy-dist

# Instalar e configurar agentes no lab
scripts/lab-simulacao.sh install node-direct aiceberg-agent-linux-amd64.tar.gz
scripts/lab-simulacao.sh install node-hub aiceberg-agent-linux-amd64.tar.gz
scripts/lab-simulacao.sh install node-relay aiceberg-agent-linux-amd64.tar.gz

scripts/lab-simulacao.sh configure node-direct direct TOKEN_DIRECT https://api.aiceberg.com.br
scripts/lab-simulacao.sh configure node-hub hub TOKEN_HUB https://api.aiceberg.com.br
scripts/lab-simulacao.sh configure node-relay relay TOKEN_RELAY https://api.aiceberg.com.br http://node-hub:9090 TOKEN_HUB

scripts/lab-simulacao.sh start node-hub
scripts/lab-simulacao.sh start node-relay
scripts/lab-simulacao.sh start node-direct

# Ver estado/logs
scripts/lab-simulacao.sh status node-hub
scripts/lab-simulacao.sh agent-logs node-hub
```

Encerrar lab:

```bash
scripts/lab-simulacao.sh down
```

Por padrão o wrapper usa `/Users/brenomayder/projects/simulacao/aiceberg_lab`.  
Se mudar o caminho, use `AICEBERG_LAB_ROOT=/novo/caminho`.

---

## 🚀 Execução com vínculo por token (modo direto)

Enquanto não temos instalador, use este fluxo para rodar localmente com token do painel:

1. Gere um token no painel (agente pendente).
2. Na primeira execução, passe o token (será persistido em `./data/agent.token` e `./data/bootstrap.ok`):
   ```bash
   API_BASE_URL=http://127.0.0.1:8082 \
   AGENT_TOKEN=SEU_TOKEN_AQUI \
   ./scripts/dev-run.sh
   ```
   Se a API retornar que o token já foi usado (409), você pode pular o bootstrap criando os arquivos manualmente:
   ```bash
   mkdir -p data
   echo -n "SEU_TOKEN_AQUI" > data/agent.token
   echo '{"token":"SEU_TOKEN_AQUI"}' > data/bootstrap.ok
   chmod 600 data/agent.token data/bootstrap.ok
   ```
3. Nas próximas execuções, basta:
   ```bash
   ./scripts/dev-run.sh
   ```
   O agente lerá o token/estado persistido, pulará bootstrap e enviará telemetria com `Authorization: Token <token>`.

## 🧱 Gerar instaladores
1. Garanta que `API_BASE_URL` está apontando para `https://api.aiceberg.com.br` (o agente já adiciona `/v1/...` internamente).
2. Execute os comandos:
   ```bash
   chmod +x scripts/build_installers.sh
   ./scripts/build_installers.sh
   ls dist
   ```
3. Os artefatos saem em `dist/` (tar.gz/zip com binário, `README_INSTALL.txt`, service/PS1 e `agent.env.example`). Publique esses arquivos no painel conforme o SO do usuário.
   > Dica: `go build` manual com `GOOS=...` só recompila o binário; use o script acima para gerar o pacote completo (binário + scripts + README + compressão).
4. Cada README do pacote instrui sobre como definir `AGENT_TOKEN`/`AGENT_TOKEN_PATH` e instalar o serviço (systemd/launchd/Windows).

Para o passo a passo de instalação por SO e por modo (direct/hub/relay), veja `docs/guia_instalacao.md`.

Para homologar coleta Agentless SNMP com perfis proprietarios ALE/OAW enviados pela web, veja `docs/pkg35_agentless_snmp_vendor_profiles.md`.

Para Kubernetes, use o manifest `deploy/kubernetes/aiceberg-agent.yaml` ou o chart `deploy/helm/aiceberg-agent`. O coletor `kubernetes` usa ServiceAccount com RBAC somente leitura para nodes, pods e events; veja `docs/pkg65_kubernetes.md`.

Para checks locais extensíveis, habilite `LOCAL_CHECKS_ENABLED=true` e configure `local_checks` remoto ou `LOCAL_CHECKS_JSON`. O runtime usa allowlist e nao executa shell generico; veja `docs/pkg66_local_checks.md`.

Para operação de frota, `inspect_runtime_config` expõe `fleet_runtime` e o comando permitido `collect_support_flare` coleta evidência sanitizada para suporte; veja `docs/pkg67_fleet_rollout_flare.md`.

Para endurecer config remota e update, configure `REMOTE_CONFIG_SIGNATURE_SECRET` e, quando pronto para enforcement, `REMOTE_CONFIG_SIGNATURE_REQUIRED=true`. Veja `docs/pkg68_security_hardening.md`.

Notas:
- API de produção é o padrão (`https://api.aiceberg.com.br`) e o agente junta `/v1/...` sozinho; use `API_BASE_URL` apenas para apontar para ambientes de teste.
- Bootstrap (`POST /v1/agent/bootstrap`) já envia `versao_agente` com `internal/common/version.Version`, então a API acompanha qual versão do agente cada host executa.
- Modos de conexão: `AGENT_MODE=direct` (padrão, envia para API), `AGENT_MODE=hub` (recebe `/v1/ingest` via `HUB_LISTEN_ADDR` e reenvia à API) e `AGENT_MODE=relay` (envia para `HUB_URL`, sem falar direto com a API). `SKIP_BOOTSTRAP=true` pode ser usado em relay puro.
- Coleta de logs (SOC inicial): habilite com `OSLOG_ENABLED=true` e liste arquivos em `OSLOG_FILES` (ex.: `/var/log/auth.log,/var/log/syslog`); os eventos são enviados em lotes próprios para `/v1/logs/raw`, com cursor persistido em `OSLOG_CURSOR_PATH`.
- Endpoint de bootstrap usado: `POST /v1/agent/bootstrap` (header `Authorization: Token <token>`).
- Saúde local: `http://localhost:8081/health` (configurável via `HEALTH_PORT`).
- Ping remoto: o agente faz long-polling em `/v1/agent/ping` a cada `PING_INTERVAL` segundos (default 5s); ao receber um desafio `{challenge}`, responde com `POST /v1/agent/ping` incluindo hostname, versão e timestamp.
- Configuração remota: o agente puxa `/v1/agent/config` a cada `CONFIG_SYNC_INTERVAL` (default 30s), salva em `PREFS_PATH` (default `./data/collect_prefs.json`) e passa a coletar somente o que estiver marcado; o payload retornado deve conter os flags de coleta e uma `version` para evitar reprocesso.
- Self-healing: o agente consulta a fila de comandos em `SELFHEAL_POLL_INTERVAL` segundos (default 30), executa somente comandos da allowlist segura (`restart_agentless_worker`, `reload_configuration`, `clear_local_lock`, `requeue_pending_collect`, `validate_api_connectivity`, `resync_clock`, `inspect_runtime_config`, `collect_support_flare`, `apply_agent_mode`) e reporta status para auditoria operacional na web. Shell arbitrário (`shell`, `exec`, `bash`, `cmd`, `powershell` etc.) é bloqueado.
- Troca remota de modo: `apply_agent_mode` persiste `direct|hub|relay` no override local `AGENT_MODE_OVERRIDE_PATH` (default ao lado de `PREFS_PATH`) e agenda restart controlado por saída do próprio processo; em serviços com `Restart=always`, o gerenciador sobe o agente já lendo o modo novo.
- Auto-update remoto (opcional): se `AUTO_UPDATE_ENABLED=true`, o payload de `/v1/agent/config` pode incluir `update = { version, url, sha256, force }`. O agente baixa o artefato e executa `AUTO_UPDATE_COMMAND` com `AICEBERG_UPDATE_FILE` e variáveis relacionadas.
  - Linux recomendado: `AUTO_UPDATE_COMMAND=/usr/local/sbin/aiceberg-agent-update-launcher.sh` (script desacoplado que dispara `aiceberg-agent-apply-update.sh` via `systemd-run`, com lock e restart seguro do serviço).
- A coleta envia um pacote único (`metric/sub=sysmetrics`) com CPU, memória, disco (I/O + SMART), rede, host, sensores/fans, bateria, GPU (NVIDIA), serviços, time sync (NTP), sanity (ping/DNS), backlog da fila, logs (.log em ./logs), updates (apt/softwareupdate), top processos.

Quando formos criar instaladores, este fluxo servirá de base: validar token, gravar localmente e evitar reuso.

---

## 📄 Licença

Projeto de propriedade do **AIceberg**, desenvolvido sob orientação do Arquiteto do Caos Elegante.  
Todos os direitos reservados.

---

## Modelo de trabalho IA - 2026-05-22

Projeto: `aiceberg_agent`. Stack: Go agent/CLI, gopsutil, NTP, SNMP. Este arquivo segue `PATHS.toml` e deve ser mantido atualizado ao iniciar ou alterar o projeto.

## Modelo de trabalho

- `MAPA_MENTAL_MARKMAP.md` contem o mapa completo para Markmap; `MAPA_EXECUTIVO_MARKMAP.md` contem a versao resumida.
