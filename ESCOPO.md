# ESCOPO.md

Escopo funcional e técnico do projeto.

## Produto

- Nome:
- Público-alvo:
- Problema que resolve:
- Resultado esperado:

## Módulos

| Módulo | Objetivo | Stack | Status |
|---|---|---|---|
|  |  |  |  |

## Arquitetura real do projeto

Preencher após inspeção do projeto existente.

- Linguagem principal:
- Frameworks:
- Estrutura atual de pastas:
- Convenções saudáveis existentes:
- Pontos que violam SOLID ou dificultam teste:
- Boundaries reais:
- Estruturas que devem ser preservadas:
- Estruturas que devem ser corrigidas incrementalmente:

## Nomenclatura oficial do projeto

Preencher após inspeção da stack e dos padrões existentes.

| Conceito | Nome usado no projeto | Onde fica | Observação |
|---|---|---|---|
| Entrada HTTP ou equivalente |  |  | Ex.: Controller, Handler, Endpoint, Route |
| Caso de uso / regra de aplicação |  |  | Ex.: Service, UseCase, Action, Interactor |
| Regra de domínio |  |  | Ex.: DomainService, Policy, Rule |
| Persistência |  |  | Ex.: Repository, DAO, ActiveRecord, Model |
| Integração externa |  |  | Ex.: Adapter, Client, Gateway, Provider |
| Payload de entrada |  |  | Ex.: Request, Input, Schema, FormRequest |
| Payload de saída |  |  | Ex.: Response, DTO, Resource, Output |
| Validação |  |  | Ex.: Validator, Schema, FormRequest |
| Regra variável |  |  | Ex.: Policy, Strategy, Rule |
| Modelo de domínio |  |  | Ex.: Entity, DomainModel, Model |
| Modelo de tela |  |  | Ex.: ViewModel, Presenter, UIModel |

## Contratos de módulo

Use um bloco por módulo relevante.

```txt
### Módulo: nome

Responsabilidade:
Onde fica:
Camadas reais:
Entradas:
Saídas:
Dependências permitidas:
Dependências proibidas:
Regras que pertencem ao módulo:
Regras que não pertencem ao módulo:
Testes obrigatórios:
Riscos:
Rollback:
```

## Arquitetura alvo

- Backend/API:
- Web:
- App:
- Banco:
- Fila/cache:
- Integrações:

## Princípios inegociáveis

- SOLID.
- Arquivos pequenos e coesos.
- CRUD separado por responsabilidade.
- Testes proporcionais ao risco.
- Sem migrations; banco por `.sql` quando existir.
- Sem segredo versionado.

## Fora de escopo

-

## Regras de produto

-

## Integrações

| Integração | Tipo | Risco | Observações |
|---|---|---|---|
|  |  |  |  |

## Dados

- Banco:
- Scripts SQL:
- Dados sensíveis:
- Retenção:

## Critérios de sucesso

-

## Decisões

Decisões arquiteturais, técnicas e operacionais ficam em `DECISOES.md`.

## Adaptacao deste projeto

- Projeto: `aiceberg_agent`
- Tipo: Agente Go
- Stack detectada: Go agent/CLI, gopsutil, NTP, SNMP
- Regra: adaptar comandos, camadas, testes, fixtures e limites conforme a realidade deste modulo Go.
