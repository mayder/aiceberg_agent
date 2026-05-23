# DEMANDAS.md

Backlog executável do projeto.

Regras transversais ficam em `GOVERNANCA.md`. Workflow, SOLID, arquitetura e Definition of Done ficam em `QUALITY_ROADMAP.md`.

## Como usar

1. Ler `PATHS.toml`.
2. Ler `QUALITY_ROADMAP.md`.
3. Ler `GOVERNANCA.md`.
4. Ler o pacote completo neste arquivo.
5. Implementar por lote pequeno.
6. Rodar validação rasa ao concluir cada lote.
7. Atualizar este arquivo ao concluir lote ou registrar bloqueio real.
8. Rodar validação completa quando o pacote estiver 100%.
9. Fazer commit ao fechar pacote 100%.

## Regras de execução por pacote

- Não implementar item fora do pacote sem registrar dependência ou novo lote.
- Não criar migration; banco entra em `.sql`.
- Não usar nomes `PKG-XX` em runtime, banco, payload público, logs estruturados ou UI.
- Todo pacote com UI precisa validar `TELAS.md`.
- Todo pacote que altera contrato precisa ajustar consumidores afetados ou documentar compatibilidade temporária.
- Todo pacote deve explicitar testes esperados.
- Cada lote deve registrar validação rasa.
- Pacote só fecha com validação completa.
- Pacote fechado deve gerar commit, sem obrigação de push.

## Quando criar pacote

Crie ou use um pacote quando a mudança:

- entrega fluxo novo;
- altera regra de negócio;
- altera contrato público;
- mexe em banco;
- toca múltiplas telas, módulos ou apps;
- muda permissão, segurança, sincronização ou integração;
- exige vários lotes;
- precisa de validação completa no fechamento.

Não precisa criar pacote quando a mudança:

- é só documental;
- corrige label, copy ou texto simples;
- ajusta estilo local de baixo risco;
- corrige bug simples e isolado;
- não altera contrato, regra, persistência, permissão ou integração.

Se uma melhoria simples crescer, criar pacote antes de continuar.

## Organização por pacotes entregáveis

| Pacote | Nome | Prioridade | Status |
|---|---|---:|---|
| `PKG-01` | Fundação do projeto e governança | P0 | modelo |
| `PKG-02` | Primeiro fluxo vertical validável | P0 | exemplo |

## Modelo de pacote

```txt
## [PKG-XX] Nome do pacote

Status:
Prioridade:
Tipo:
Módulos afetados:

### Objetivo

### Escopo

### Fora de escopo

### Lotes

- [ ] Lote 1:
  - Validação rasa:
- [ ] Lote 2:
  - Validação rasa:
- [ ] Lote 3:
  - Validação rasa:

### Contratos

### Contrato de módulo

- Responsabilidade:
- Entradas:
- Saídas:
- Dependências permitidas:
- Dependências proibidas:
- Testes obrigatórios:

### Decisões de arquitetura

- Registrar em `DECISOES.md` quando houver decisão relevante.

### Dívida técnica

- Item:

### Telas afetadas

### Testes obrigatórios

### Observabilidade

- Logs técnicos:
- Auditoria:
- Métricas:
- Reutiliza estrutura existente:
- Nova tabela necessária:
- Justificativa:
- Retenção:
- Cleanup:

### Validação completa de fechamento

### Review de fechamento

- [ ] Escopo respeitado.
- [ ] Todos os lotes concluídos ou bloqueios reais documentados.
- [ ] Sem mudança fora do pacote sem justificativa.
- [ ] SOLID e boundaries respeitados.
- [ ] Arquivos grandes avaliados.
- [ ] Contratos e consumidores sincronizados.
- [ ] Testes por criticidade executados.
- [ ] Bugs relacionados retestados.
- [ ] Regressões dos lotes anteriores verificadas.
- [ ] Docs oficiais atualizadas quando aplicável.
- [ ] SQL com rollback quando aplicável.
- [ ] Observabilidade crítica revisada quando aplicável.
- [ ] Retenção/cleanup de logs ou auditoria definidos quando houver persistência.
- [ ] Branch conferida.
- [ ] Diff final revisado.
- [ ] `./check.sh` passou.

### Critério de aceite

### Risco e rollback

### Commit de fechamento

- Hash:
- Mensagem:
- Check:
```

## [PKG-01] Fundação do projeto e governança

Status: modelo.
Prioridade: P0.
Tipo: documentação + estrutura.

### Objetivo

Criar a base governável do projeto antes de iniciar implementação funcional.

### Escopo

- `PATHS.toml`.
- `ESCOPO.md`.
- `GOVERNANCA.md`.
- `QUALITY_ROADMAP.md`.
- `DEMANDAS.md`.
- `BUGS.md`.
- `TELAS.md`.
- `TESTES.md`.
- `RUNBOOK.md`.
- `README.md`.
- `check.sh`.
- `MAPA_MENTAL_MARKMAP.md`.

### Fora de escopo

- Regra de negócio real.
- Banco de produção.
- Publicação.

### Lotes

- [ ] Ajustar `PATHS.toml` com paths reais.
- [ ] Preencher `ESCOPO.md`.
- [ ] Ajustar governança do projeto.
- [ ] Ajustar playbooks técnicos no roadmap.
- [ ] Criar backlog real em `DEMANDAS.md`.
- [ ] Mapear telas reais em `TELAS.md`.
- [ ] Definir testes reais em `TESTES.md`.
- [ ] Definir runbook real.
- [ ] Rodar `./check.sh`.

### Critério de aceite

- Arquivos obrigatórios existem.
- `./check.sh` passa.
- Escopo, governança e backlog não estão misturados.
- Commit de fechamento realizado.

### Validação

- Rodar `./check.sh`.

### Risco e rollback

- Risco baixo. Reverter commit do scaffold se a estrutura precisar ser refeita.

## [PKG-02] Primeiro CRUD ou fluxo vertical validável

Status: exemplo.
Prioridade: P0.
Tipo: produto + arquitetura + testes.

### Objetivo

Entregar um fluxo pequeno, real e testável, respeitando SOLID.

### Escopo

- API ou camada equivalente.
- Listagem e filtros.
- Detalhamento.
- Cadastro.
- Edição.
- Formulário compartilhado quando fizer sentido.
- Testes unitários e de contrato.
- Atualização de `TELAS.md`.

### Fora de escopo

- Otimizações avançadas.
- Redesign amplo.
- Automação E2E completa sem fluxo estabilizado.

### Lotes

- [ ] Definir contrato e modelo de domínio.
- [ ] Criar persistência/adapters.
- [ ] Criar services/use cases.
- [ ] Criar endpoints/handlers finos.
- [ ] Criar listagem e filtros.
- [ ] Criar detalhe.
- [ ] Criar cadastro.
- [ ] Criar edição.
- [ ] Criar formulário compartilhado sem regra de rota.
- [ ] Criar testes.
- [ ] Atualizar `TELAS.md` e `TESTES.md`.

### Critério de aceite

- CRUD não está concentrado em uma única tela.
- Regra de negócio não está na UI.
- `./check.sh` passa.
- Commit de fechamento realizado.

### Risco e rollback

- Reverter pacote por lote se o contrato ficar errado.

## Adaptacao deste projeto

- Projeto: `aiceberg_agent_poc`
- Tipo: POC Go
- Stack detectada: Go POC, gopsutil
- Regra: adaptar comandos, camadas, testes, fixtures e limites conforme a realidade deste modulo Go.
