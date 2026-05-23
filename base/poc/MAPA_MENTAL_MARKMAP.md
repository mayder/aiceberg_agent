---
title: IA para desenvolvimento de software com SOLID
markmap:
  colorFreezeLevel: 2
  maxWidth: 320
---

# IA para desenvolvimento de software com SOLID

## Objetivo

### Menos tokens

- Ler `PATHS.toml` primeiro
- Identificar tipo de tarefa
- Ler somente arquivos necessários
- Usar playbook certo
- Evitar redescobrir padrão a cada demanda

### Mais qualidade

- SOLID obrigatório
- Mudanças pequenas
- Separação de responsabilidades
- Testes proporcionais ao risco
- Check automatizado
- Validação rasa por lote
- Validação completa por pacote

### Menos manutenção

- Arquivos coesos
- Sem monólitos gigantes
- Contratos explícitos
- Backlog vivo
- Bugs rastreáveis

## Arquivos oficiais

### `PATHS.toml`

- Caminhos oficiais
- Módulos
- Checks
- Ordem de leitura
- `quality.runtime_dirs`
- `quality.stack`
- `quality.fixtures`

### `ESCOPO.md`

- Produto
- Módulos
- Contratos de módulo
- Arquitetura real
- Boundaries reais
- Nomenclatura oficial
- Stack
- Fora de escopo
- Princípios inegociáveis
- Decisões de arquitetura

### `QUALITY_ROADMAP.md`

- SOLID
- Workflow
- Playbooks por tarefa
- Templates mínimos
- Check por stack
- Definition of Done
- Limites de manutenção

### `GOVERNANCA.md`

- Gates de release
- Prioridades
- Riscos
- Rollback
- Métricas mínimas

### `DEMANDAS.md`

- Pacotes
- Lotes
- Critérios de aceite
- Testes esperados
- Status real
- Commit de fechamento

### `TELAS.md`

- Listagem e filtros
- Detalhamento
- Cadastro
- Edição
- Estados de UI

### `TESTES.md`

- Unitário
- Service/use case
- Repository/adapter
- Contrato/API
- Componente/widget
- E2E
- Cobertura por criticidade
- Percentual opcional
- Fixtures
- Seeds
- Reteste de bug
- Raso por lote
- Completo por pacote
- Proporcional por bug
- Local para melhoria simples

### `DECISOES.md`

- Decisões técnicas
- Decisões arquiteturais
- Trade-offs
- Alternativas consideradas
- Como reverter

### `BUGS.md`

- Reprodução
- Severidade
- Causa provável
- Hipóteses alternativas
- Menor correção
- Reteste

### `RUNBOOK.md`

- Rodar local
- Diagnóstico
- Observabilidade
- Deploy
- Smoke test
- Rollback
- Incidente

## SOLID

### Single Responsibility

- Uma responsabilidade por arquivo
- Uma razão principal para mudar
- UI não contém regra de negócio
- Handler não contém regra de negócio
- Repository não decide negócio

### Open Closed

- Extensão antes de alteração arriscada
- Estratégias para variações reais
- Policies para regras variáveis
- Adapters para dependências externas

### Liskov Substitution

- Fakes respeitam contratos
- Adapters são substituíveis
- Mocks não mentem sobre comportamento
- Testes protegem contrato

### Interface Segregation

- Interfaces pequenas
- Contratos específicos
- UI consome modelo de tela
- Domínio consome contrato de domínio

### Dependency Inversion

- Domínio não depende de framework
- Regra não depende de banco
- Regra não depende de HTTP
- Infra depende do domínio

## CRUD correto

### Listagem e filtros

- Busca
- Filtros
- Paginação
- Ordenação
- Estado vazio
- Ações de linha
- Não carrega detalhe completo de todos

### Detalhamento

- Leitura completa
- Metadados
- Histórico quando existir
- Ações contextuais
- Permissões

### Cadastro

- Novo registro
- Validação
- Submissão
- Sucesso
- Erro
- Cancelamento

### Edição

- Carregar registro
- Validar alteração
- Tratar conflito
- Salvar
- Confirmar sucesso
- Cancelar

### Formulário compartilhado

- Permitido para cadastro e edição
- Sem regra de rota
- Sem carregamento remoto
- Sem persistência direta
- Apenas campos, validação local e eventos

## Playbooks da IA

### Leitura mínima

- Orientação: `PATHS.toml`
- Pacote: roadmap, governança, demandas
- Bug: bugs e testes
- UI: telas e testes
- Arquitetura: escopo, roadmap, decisões
- Operação: governança e runbook
- Ampliar só se houver risco

### Antes de editar

- Ler `PATHS.toml`
- Ler roadmap e governança
- Ler pacote em `DEMANDAS.md`
- Confirmar branch
- Perguntar antes se for `main` ou `hml` sem autorização explícita
- Inspecionar código real
- Mapear arquitetura real
- Preservar convenções saudáveis
- Identificar testes existentes

### Ao implementar

- Um lote por vez
- Separar camadas
- Respeitar estrutura real do projeto
- Criar contratos explícitos
- Registrar decisão arquitetural relevante
- Registrar dívida técnica se arquivo grande não puder ser quebrado
- Atualizar docs afetadas
- Evitar arquivo grande

### Ao decidir pacote

- Fluxo novo vira pacote
- Regra de negócio vira pacote
- Contrato público vira pacote
- Banco vira pacote
- Múltiplos módulos viram pacote
- Label/copy simples não precisa pacote
- Bug simples não precisa pacote

### Ao testar

- No lote: teste raso e focado
- Em documentação: revisar diff
- Em melhoria simples: teste local
- Em bug simples: reteste focado
- Em bug complexo: validação completa do fluxo
- Unitário para regra pura alterada
- Service para fluxo de negócio alterado
- Contrato para API alterada
- Componente para UI crítica alterada
- No pacote: suíte completa relevante
- E2E no fechamento ou regressão crítica
- P0 exige regra, contrato, permissão/erro e fluxo principal
- P1 exige regra/contrato/UI crítica quando aplicável
- P2 exige teste local ou revisão de diff

## Cobertura

### Por criticidade

- P0 crítico
- P1 importante
- P2 baixo risco
- Evidência obrigatória
- Não depende só de percentual

### Percentual opcional

- Configurado em `PATHS.toml`
- Útil quando ferramenta é estável
- Não deve ser única métrica
- Preferir meta incremental em legado

## Fixtures e dados de teste

### Regras

- Sem dado real instável
- Sem segredo
- Sem dump grande de produção
- IDs determinísticos
- Data fixa ou clock controlado
- Teste isolado

### Por tipo

- Unitário em memória
- Service com fake/mock
- Repository com banco local ou fixture controlada
- API com payload versionado
- E2E com seed controlado

### Ao fechar

- Atualizar `DEMANDAS.md`
- Atualizar `TELAS.md` se UI mudou
- Atualizar `BUGS.md` se bug foi corrigido
- Rodar `./check.sh`
- Fazer review de fechamento
- Corrigir regressões dos lotes anteriores
- Fazer commit do pacote
- Responder curto: feito, validação, bloqueios

## Review de fechamento

### Conferir

- Escopo
- Lotes
- Mudanças fora do pacote
- SOLID
- Boundaries
- Arquivos grandes
- Contratos
- Testes
- Bugs
- Regressões
- Docs
- SQL e rollback
- Branch
- Diff final
- Check
- Commit limpo

## Bug e melhoria fora de pacote

### Documentação

- Sem código runtime
- Revisar diff
- Check documental quando aplicável
- Sem E2E

### Melhoria simples

- Label
- Copy
- Ícone
- Espaçamento local
- Mensagem local
- Teste local/focado

### Bug simples

- Causa clara
- Impacto local
- Menor correção
- Reteste focado

### Bug complexo

- Regra de negócio
- Contrato
- Banco
- Permissão
- Segurança
- Integração
- Múltiplos módulos
- Teste automatizado quando viável
- Check completo

## Pacote e lote

### Lote

- Subentrega pequena
- Pode resolver uma ou várias demandas
- Validação rasa
- Atualiza `DEMANDAS.md`
- Sem commit obrigatório

### Pacote 100%

- Todos os lotes concluídos
- Validação completa
- Regressão dos lotes anteriores
- Correção de pendências
- Commit obrigatório
- Push não obrigatório

## Contrato de módulo

### Deve definir

- Responsabilidade
- Onde fica
- Camadas reais
- Entradas
- Saídas
- Dependências permitidas
- Dependências proibidas
- Testes obrigatórios
- Riscos
- Rollback

## Nomenclatura oficial

### Registrar por projeto

- Entrada HTTP
- Caso de uso
- Regra de domínio
- Persistência
- Integração externa
- Payload de entrada
- Payload de saída
- Validação
- Regra variável
- Modelo de domínio
- Modelo de tela

### Regras

- Não inventar nome novo sem motivo
- Não misturar termos para o mesmo papel
- Preservar padrão saudável existente
- Mudança de padrão exige `DECISOES.md`

## Decisões e dívida técnica

### Decisão

- Contexto
- Decisão
- Alternativas
- Consequências
- Como reverter

### Dívida técnica

- Sintoma
- Risco
- Arquivo
- Impacto
- Proposta incremental

## Banco de dados

### Sem migrations

- `.sql` versionado
- Idempotente quando possível
- Ordem de execução
- Impacto
- Rollback
- Validação

### Cuidados

- Sem `PKG-XX` em runtime
- Sem nomes internos em banco
- Backfill separado
- Cleanup separado
- Mudança irreversível pede confirmação

## Observabilidade da aplicação

### Quando aplicar

- Login
- Permissão
- Pagamento
- Dados sensíveis
- Status crítico
- Jobs
- Integrações
- Deploy
- Ações administrativas

### Registrar

- Evento
- Nível
- Request/correlation/job id
- Usuário/cliente quando existir
- Entidade afetada
- Resultado
- Erro resumido

### Não registrar

- Senha
- Token
- Segredo
- Payload sensível completo
- Dado pessoal desnecessário

### Retenção e limpeza

- Preferir estrutura existente
- Evitar tabela nova
- Nova tabela exige justificativa
- Retenção definida
- Cleanup automático

## Garantias

### Check automatizado

- Arquivos obrigatórios
- `PATHS.toml` coerente
- Markdown íntegro
- SOLID documentado
- CRUD separado
- Testes definidos
- Segredos óbvios bloqueados
- Arquivos grandes detectados
- Runtime sem `PKG-XX`
- Scripts pequenos em `scripts/`
- Stack adaptável por projeto
- Fixtures validadas quando configuradas
- Observabilidade crítica com retenção
- Camadas validadas quando configuradas

### Gates

- Check verde
- Branch protegida confirmada
- Sem bug crítico aberto
- Review de fechamento
- Fluxo validado
- Rollback claro
- Docs sincronizadas
- Commit por pacote

### Resultado

- IA com menos ambiguidade
- Código menor
- Responsabilidades claras
- Testes melhores
- Manutenção viável

## Adaptacao deste projeto

- Projeto: `aiceberg_agent_poc`
- Tipo: POC Go
- Stack detectada: Go POC, gopsutil
- Regra: adaptar comandos, camadas, testes, fixtures e limites conforme a realidade deste modulo Go.
