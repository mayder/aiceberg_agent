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

## 🧭 Próximos Passos

1. Implementar o módulo `sysmetrics` (coleta de CPU, RAM, disco, rede).  
2. Criar camada de transporte HTTP (`httpjson`) com compressão e idempotência.  
3. Integrar fila local (`bbolt`) e lógica de reenvio automático.  
4. Validar ingestão no backend do AIceberg.  
5. Atualizar o documento [Arquitetura.md](/base/readme/Arquitetura.md) com os diagramas e decisões tomadas.

---

## ⚙️ Execução Rápida

```bash
make tidy
make run
```

O agente carregará o arquivo de configuração `./configs/config.example.yml`, exibirá logs básicos e iniciará o ciclo de coleta e envio (modo esqueleto).

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
4. Cada README do pacote instrui sobre como definir `AGENT_TOKEN`/`AGENT_TOKEN_PATH` e instalar o serviço (systemd/launchd/Windows).

Notas:
- API de produção é o padrão (`https://api.aiceberg.com.br`) e o agente junta `/v1/...` sozinho; use `API_BASE_URL` apenas para apontar para ambientes de teste.
- Bootstrap (`POST /v1/agent/bootstrap`) já envia `versao_agente` com `internal/common/version.Version`, então a API acompanha qual versão do agente cada host executa.
- Modos de conexão: `AGENT_MODE=direct` (padrão, envia para API), `AGENT_MODE=hub` (recebe `/v1/ingest` via `HUB_LISTEN_ADDR` e reenvia à API) e `AGENT_MODE=relay` (envia para `HUB_URL`, sem falar direto com a API). `SKIP_BOOTSTRAP=true` pode ser usado em relay puro.
- Endpoint de bootstrap usado: `POST /v1/agent/bootstrap` (header `Authorization: Token <token>`).
- Saúde local: `http://localhost:8081/health` (configurável via `HEALTH_PORT`).
- Ping remoto: o agente faz long-polling em `/v1/agent/ping` a cada `PING_INTERVAL` segundos (default 5s); ao receber um desafio `{challenge}`, responde com `POST /v1/agent/ping` incluindo hostname, versão e timestamp.
- Configuração remota: o agente puxa `/v1/agent/config` a cada `CONFIG_SYNC_INTERVAL` (default 30s), salva em `PREFS_PATH` (default `./data/collect_prefs.json`) e passa a coletar somente o que estiver marcado; o payload retornado deve conter os flags de coleta e uma `version` para evitar reprocesso.
- A coleta envia um pacote único (`metric/sub=sysmetrics`) com CPU, memória, disco (I/O + SMART), rede, host, sensores/fans, bateria, GPU (NVIDIA), serviços, time sync (NTP), sanity (ping/DNS), backlog da fila, logs (.log em ./logs), updates (apt/softwareupdate), top processos.

Quando formos criar instaladores, este fluxo servirá de base: validar token, gravar localmente e evitar reuso.

---

## 📄 Licença

Projeto de propriedade do **AIceberg**, desenvolvido sob orientação do Arquiteto do Caos Elegante.  
Todos os direitos reservados.
