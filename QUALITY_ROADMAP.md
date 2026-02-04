# Qualidade, Arquitetura e Roadmap (Aiceberg Agent)

Siga o QUALITY_ROADMAP.md como regra mestra e rode ./check.sh no final.

Escopo: agente Go (cmd/, internal/, configs/, scripts/).

## Regra Mestra (Obrigatoria)

1. Este arquivo e a fonte unica de padrao e qualidade do agente.
2. Toda solicitacao nova deve seguir as regras e o processo descritos aqui.
3. Em caso de duvida ou conflito, este arquivo vence.

## Processo Obrigatorio (Toda Mudanca)

1. Respeitar camadas e responsabilidades: cmd/ (entrypoints), internal/domain (regras), internal/interfaces (ports), internal/data (repos), internal/platform (infra/transport), internal/bootstrap.
2. Payloads devem ser validados e mapeados antes de uso.
3. Regras de negocio isoladas em services/policies no dominio.
4. Logs estruturados em fluxos criticos (event_id, agent_id, route, job).
5. Tratamento defensivo de erros e retries com backoff quando aplicavel.
6. Testes minimos para handlers/servicos alterados.
7. ./check.sh deve passar antes de qualquer commit.

## Padrao Unico de Logs

1. Sempre usar `logger.KV("mensagem", "chave", valor, ...)`.
2. Campos obrigatorios por contexto:
Envelopes: `event_id`, `agent_id`, `route`.
HTTP/transport: `route`, `batch_size`, `duration_ms`, `err`.
Agentless: `job_id`, `job_type`, `status`, `latency_ms`, `err`.
Config/prefs: `version`, `err`.
3. Mensagem base curta e estavel (ex.: `collect buffered`, `transport failed`).
4. Debug em arquivo: quando `LOG_LEVEL=debug`, os logs tambem sao gravados em `LOG_FILE_PATH` (default `./data/agent.debug.log`).
5. Rotacao por tamanho: `LOG_FILE_MAX_MB` (default 10) e `LOG_FILE_MAX_BACKUPS` (default 3).

## Definition of Done (Checklist Obrigatorio)

1. Camadas respeitadas e imports corretos.
2. Validacao e mapeamento aplicados.
3. Logs estruturados atualizados quando necessario.
4. Testes relevantes criados/ajustados.
5. ./check.sh rodou e passou sem erros.

## Automacao do Check (Sem Commit com Erro)

1. Hook pre-commit obrigatorio: .githooks/pre-commit roda ./check.sh.
2. Ativacao obrigatoria do hook no repositorio.
3. Execute git config core.hooksPath .githooks uma vez na maquina.
4. Se o hook nao estiver ativo, nao e permitido commitar.

## Roadmap local

- [x] Definir check.sh (go test + lint)
- [x] Criar .githooks/pre-commit para bloquear commit sem check
- [x] Padronizar contratos de interfaces (ports)
- [x] Consolidar observabilidade (logs + metricas)

## Como rodar checks (atalhos)

- ./check.sh
