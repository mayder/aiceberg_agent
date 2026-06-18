# DECISOES.md

Registro de decisões arquiteturais, técnicas e operacionais relevantes.

Este arquivo existe para evitar que decisões importantes fiquem espalhadas em backlog, escopo, comentários ou histórico de chat.

## Quando registrar

- Escolha de stack, biblioteca, provider ou framework.
- Criação de abstração.
- Mudança de boundary.
- Exceção a padrão.
- Decisão de não refatorar agora.
- Trade-off de prazo, risco, performance ou compatibilidade.
- Mudança de estratégia de teste, deploy ou rollback.
- Decisão que a IA provavelmente rediscutiria no futuro se não estivesse documentada.

## Regras

- Decisão registrada deve ser curta e objetiva.
- Não registrar decisão trivial.
- Se uma decisão substituir outra, marcar a anterior como `substituída`.
- Se uma decisão for revertida, registrar motivo e forma de reversão.
- Decisões aceitas devem ser consideradas fonte de verdade até serem substituídas ou revertidas.

## Modelo

```txt
### DEC-YYYYMMDD-01 - Título

Status: proposta | aceita | substituída | revertida
Data:
Contexto:
Decisão:
Alternativas consideradas:
Consequências:
Impacto em testes:
Impacto em rollback:
Como reverter:
Referências:
```

## Decisões

### DEC-20260618-01 - Matriz evidencial para evolucao Datadog-like do agente

- Status: aceita
- Contexto: a evolucao do agente para capacidades Datadog-like envolve mudancas coordenadas no `aiceberg_agent` e no `aiceberg_web`, com risco de quebrar snapshots, ingestao, canal, update remoto e Agentless HUB.
- Decisao: usar a matriz unica do web em `docs/agente_datadog_paridade.md` e manter neste repo o inventario tecnico `docs/pkg58_inventario_agente_atual.md`. Cada pacote de runtime deve preservar contratos atuais, introduzir flags/configs de rollback e preencher evidencia por ambiente antes de declarar paridade.
- Alternativas consideradas: manter backlog local solto ou implementar capacidades por demanda sem matriz.
- Consequencias: PKG-59 a PKG-72 precisam apontar dependencias, validacao e rollback antes de fechar.
- Impacto em testes: PKG-58 exige validacao documental e checks dos dois repos; runtime novo exige testes por pacote.
- Impacto em rollback: nenhum runtime novo e ativado por esta decisao.
- Como reverter: corrigir ou remover os documentos do PKG-58 antes de iniciar pacote dependente.

### DEC-20260618-02 - Runtime v2 compativel por metadados aditivos

- Status: aceita
- Contexto: PKG-59 precisa criar base para Collector, Forwarder, Scheduler, Supervisor e ExtensionRuntime sem quebrar os coletores legados nem os snapshots do web.
- Decisao: criar `internal/domain/runtime` com contratos internos e marcar o pipeline atual como `2-compatible`. `CollectAndBuffer` adiciona metadados opcionais ao corpo JSON (`schema_version`, `agent_pipeline_version`, `collector_name`, `ingest_endpoint`) preservando as secoes antigas. O health local e o snapshot remoto expõem a versao do pipeline e o scheduler snapshot.
- Alternativas consideradas: refatorar todo `app.Run` de uma vez ou criar um runtime paralelo. Ambas aumentariam risco de regressao.
- Consequencias: novos coletores podem migrar para o contrato v2 sem alterar endpoints atuais.
- Impacto em testes: testes unitarios cobrem metadados, health e snapshot de runtime.
- Impacto em rollback: publicar a versao anterior do agente; o backend ignora campos aditivos.
- Como reverter: remover injecao de metadados e voltar `CollectAndBuffer` ao corpo original.

### DEC-20260618-03 - Logs v2 usam campos aditivos no contrato legado

- Status: aceita
- Contexto: PKG-60 precisa evoluir logs com parsing, redaction, cursor e filtros sem quebrar `/v1/logs/raw`, SOC/NOC e agentes ja instalados.
- Decisao: manter o coletor `oslogs` e adicionar campos opcionais por evento, com `schema_version=1`, `redaction_status`, atributos JSON sanitizados, origem real e contagem agregada `dropped_count`. Filtros locais aceitos por env/config remota nao persistem conteudo descartado.
- Alternativas consideradas: criar endpoint novo de logs ou persistencia nova no agente.
- Consequencias: o backend continua aceitando eventos antigos e novos; recursos avancados como OTLP, dual-shipping e listeners locais ficam em pacotes proprios.
- Impacto em testes: unitarios cobrem redaction, JSON attributes, multiline e filtros; build Windows do coletor e validado por `GOOS=windows`.
- Impacto em rollback: desativar `OSLOG_ENABLED`/flags remotas de logs ou publicar versao anterior.
- Como reverter: remover campos aditivos e filtros do coletor `oslogs`.

## Adaptacao deste projeto

- Projeto: `aiceberg_agent`
- Tipo: Agente Go
- Stack detectada: Go agent/CLI, gopsutil, NTP, SNMP
- Regra: adaptar comandos, camadas, testes, fixtures e limites conforme a realidade deste modulo Go.

## Modelo de decisao

### DEC-YYYYMMDD-01 - Titulo da decisao

- Contexto:
- Decisao:
- Alternativas consideradas:
- Impacto:
- Rollback:
