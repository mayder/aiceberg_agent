# Demandas AIceberg Agent

Backlog do agente desktop/serviço. Este arquivo complementa o backlog do `aiceberg_web` quando a entrega exigir mudanças coordenadas nos dois repositórios.

---

## Organização por pacotes entregáveis

- `PKG-32` Canal operacional bidirecional com agentes

---

## [PKG-32] Canal operacional bidirecional com agentes

**Problema a resolver** — o agente depende demais de polling HTTP para comandos, atualização, coleta sob demanda e confirmação de estado. Isso aumenta latência, gera filas grandes e dificulta diagnóstico de falhas como update, sudoers, restart e reconexão.

**Escopo no `aiceberg_agent`** — implementar o lado do agente para manter um canal operacional persistente ou quase persistente com o servidor, preservando o fluxo atual como fallback.

**Dependências** — `aiceberg_web` PKG-32, PKG-19 (self-update, self-healing, modo remoto), PKG-20 (envio incremental), configuração segura do serviço por SO.

### Lotes propostos

1) **Contrato local do canal**
   - [x] definir envelope canônico para mensagens de sessão, comando, ACK, progresso, resultado, erro e heartbeat;
   - [x] incluir `agent_id`, hostname, versão, modo, capacidades, `command_id`, `correlation_id`, timestamps e tentativas;
   - [x] garantir topologia por modo: `direct -> AIceberg`, `hub -> AIceberg`, `relay -> hub -> AIceberg`;
   - [x] preservar os contratos HTTP atuais (`ping`, `bootstrap`, `config`, `selfheal`, `update-report`, `ingest`) como compatibilidade e fallback;
   - [x] separar domínio/contrato de transporte para permitir WebSocket no MVP e polling como fallback.

2) **Cliente persistente outbound**
   - [x] criar cliente outbound para o endpoint operacional do modo `direct` (`/v1/agent/channel`, HTTP equivalente ao canal persistente);
   - [x] em modo `relay`, conectar somente ao `hub` configurado, sem exigir conexão direta com AIceberg;
   - [x] em modo `hub`, aceitar conexões dos relays e manter conexão do hub com AIceberg;
   - [x] autenticar com token atual do agente e validar rejeições de credencial;
   - [x] implementar reconnect com backoff, jitter e limite de logs;
   - [x] manter heartbeat e medição de latência.

3) **Fallback para polling atual**
   - [x] manter `/v1/agent/ping`, `/v1/agent/config` e self-healing atuais funcionando;
   - [x] ativar fallback automático quando o canal persistente cair;
   - [x] manter compatibilidade com relays que ainda operem apenas pelo fluxo atual durante rollout;
   - [x] evitar execução duplicada do mesmo comando entre canal novo e polling por idempotência local.

4) **Executor de comandos com progresso**
   - [x] padronizar ACK imediato ao receber comando;
   - [x] enviar eventos de progresso para comandos longos;
   - [x] retornar resultado final com evidência sanitizada;
   - [x] aplicar timeout, cancelamento cooperativo e retry controlado.

5) **Coleta sob demanda e streaming**
   - [x] permitir coletas sob demanda sem bloquear o loop principal;
   - [x] enviar payloads grandes em chunks com checksum/ordem;
   - [x] reportar progresso de inventário, health, network capture e agentless;
   - [x] respeitar quotas de CPU, memória, disco e rede.

6) **Update remoto com handshake**
   - [x] expor etapas de update: precheck, download, validação, apply, restart agendado, reconexão e confirmação;
   - [x] reportar falhas específicas: permissão/sudoers, pacote inválido, binário ausente, restart falho, versão não confirmada;
   - [x] confirmar versão nova após restart sem depender apenas de bootstrap manual;
   - [x] manter rollback local e fallback para o launcher atual.

7) **Segurança local**
   - [x] manter allowlist de comandos suportados;
   - [x] bloquear shell arbitrário;
   - [x] sanitizar logs, paths, tokens e payloads;
   - [x] limitar comandos destrutivos a fluxos com aprovação explícita na web.

8) **Operação e suporte**
   - [x] criar diagnóstico local do canal (`doctor` ou comando remoto equivalente);
   - [x] expor status do canal no health local;
   - [x] documentar instalação, variáveis e troubleshooting;
   - [x] adicionar testes unitários e integração local simulando servidor.

### Critérios de aceite

- [x] agente conecta outbound ao servidor e mantém presença/heartbeat;
- [x] comando simples executa via canal persistente com ACK e resultado;
- [x] queda do canal volta para polling sem perder comando pendente;
- [x] update remoto reporta etapa detalhada e confirma versão após reconexão;
- [x] payload grande é transmitido em chunks sem travar coleta principal;
- [x] `./check.sh` passa antes de qualquer release do agente.

### Fora de escopo inicial

- abrir porta inbound obrigatória no cliente;
- SSH reverso genérico;
- shell remoto arbitrário;
- conexão direta obrigatória de `relay` com AIceberg;
- remoção dos contratos HTTP atuais antes de homologação;
- remover polling atual antes de homologação e rollback.
