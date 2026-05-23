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

Ainda não há decisões registradas.

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
