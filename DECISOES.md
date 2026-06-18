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
