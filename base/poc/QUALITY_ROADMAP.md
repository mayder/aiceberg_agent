# QUALITY_ROADMAP.md

Fonte de verdade para workflow, arquitetura, SOLID, padrões de entrega e Definition of Done.

## Precedência

1. `PATHS.toml` define paths, módulos e checks oficiais.
2. Este arquivo define workflow, arquitetura, SOLID, padrões técnicos e Definition of Done.
3. `GOVERNANCA.md` define gates, prioridade, riscos e rollback.
4. `DEMANDAS.md` define backlog executável.
5. `TELAS.md` define inventário operacional de telas quando houver UI.
6. `TESTES.md` define testes automatizados, validação manual e reteste.
7. `BUGS.md` define bugs, reprodução, severidade e fechamento.

## Regra mestra para IA

Toda entrega deve ser pequena, coesa, testável, reversível e alinhada a SOLID.

A IA deve gastar tokens lendo pouco e lendo certo:

1. Sempre ler `PATHS.toml`.
2. Ler somente os arquivos oficiais relevantes para a tarefa.
3. Identificar o tipo da tarefa: pacote, lote, CRUD, API, tela web, tela app, bug, melhoria simples, documentação, SQL, teste ou operação.
4. Aplicar o playbook correspondente deste arquivo.
5. Atualizar os arquivos oficiais afetados.
6. Rodar validação proporcional ao risco.
7. Rodar validação completa e fazer commit quando o pacote fechar 100%.

## Leitura mínima por tipo de tarefa

Para reduzir tokens e acelerar resposta, a IA não deve ler todos os arquivos por padrão.

| Tipo de tarefa | Ler sempre | Ler também quando aplicável |
|---|---|---|
| Orientação inicial | `PATHS.toml` | `README.md`, `MAPA_EXECUTIVO_MARKMAP.md` |
| Pacote/lote | `PATHS.toml`, `QUALITY_ROADMAP.md`, `GOVERNANCA.md`, `DEMANDAS.md` | `TELAS.md`, `TESTES.md`, `DECISOES.md`, `RUNBOOK.md` |
| Bug simples | `PATHS.toml`, `BUGS.md`, arquivo afetado | `TESTES.md`, `TELAS.md` se houver UI |
| Bug complexo | `PATHS.toml`, `QUALITY_ROADMAP.md`, `BUGS.md`, `TESTES.md` | `GOVERNANCA.md`, `DECISOES.md`, `RUNBOOK.md` |
| Melhoria simples | `PATHS.toml`, arquivo afetado | `TELAS.md` se for UI |
| Documentação | `PATHS.toml`, arquivo de destino | arquivos citados pela alteração |
| UI/tela | `PATHS.toml`, `TELAS.md`, `TESTES.md` | `DEMANDAS.md`, `BUGS.md` |
| Testes | `PATHS.toml`, `TESTES.md` | `DEMANDAS.md`, `BUGS.md`, fixtures reais |
| Arquitetura | `PATHS.toml`, `ESCOPO.md`, `QUALITY_ROADMAP.md`, `DECISOES.md` | `GOVERNANCA.md`, código real |
| Operação/deploy | `PATHS.toml`, `GOVERNANCA.md`, `RUNBOOK.md` | `DECISOES.md`, `DEMANDAS.md` |

Se durante a execução aparecer risco, contrato, banco, segurança ou divergência de arquitetura, ampliar a leitura.

## Adaptação à arquitetura real

Este modelo não impõe nomes universais de pastas.

Antes de criar ou alterar estrutura, a IA deve:

1. Inspecionar a arquitetura real do projeto.
2. Identificar linguagem, framework, padrões existentes e convenções saudáveis.
3. Preservar a estrutura existente quando ela não viola SOLID ou segurança.
4. Documentar boundaries reais em `ESCOPO.md`, `QUALITY_ROADMAP.md` e `PATHS.toml`.
5. Ajustar `quality.runtime_dirs`, comandos de stack e scripts de validação conforme a estrutura real.
6. Só propor nova estrutura quando a atual impedir manutenção, teste, segurança ou separação de responsabilidades.

O importante é a boundary, não o nome da pasta.

Exemplos:

- Yii2 pode usar `controllers/`, `models/`, `services/`, `components/`.
- Flutter pode usar `core/`, `data/`, `domain`, `presentation/`, `features/`.
- Go pode usar `cmd/`, `internal/modules/`, `internal/platform/`.
- TypeScript pode usar `src/features/`, `src/services/`, `src/components/`, `src/types/`.

Ao aplicar este modelo em projeto existente, é proibido inventar uma estrutura nova antes de mapear e justificar a arquitetura atual.

## SOLID obrigatório

### S - Single Responsibility

- Cada arquivo, classe, função, componente, tela, handler e service deve ter uma responsabilidade clara.
- Arquivo grande deve ser dividido por motivo real de mudança.
- Proibido arquivo concentrador com múltiplos fluxos sem separação.
- Sinal de alerta: arquivo com milhares de linhas, funções que fazem validação, regra, persistência e renderização juntas.

### O - Open/Closed

- Evoluir por extensão quando o fluxo está estável.
- Evitar alterar bloco crítico grande para adicionar variação pequena.
- Estratégias, policies, mappers e adapters devem isolar variações reais.

### L - Liskov Substitution

- Implementação concreta deve respeitar o contrato da abstração.
- Adapter/repository fake, local, remoto ou mock deve se comportar de forma compatível.
- Testes devem cobrir contrato quando houver múltiplas implementações.

### I - Interface Segregation

- Interfaces devem ser pequenas e específicas.
- Não criar contrato genérico gigante para vários casos de uso.
- UI consome contrato de tela; domínio consome contrato de domínio.

### D - Dependency Inversion

- Regra de negócio não depende de framework, banco, HTTP, storage, fila ou UI.
- Camadas externas dependem do domínio, não o contrário.
- Dependência externa entra por interface, porta, adapter ou service de borda.

## Limites de tamanho e manutenção

- Função ideal: até 40 linhas, salvo transformação simples e justificada.
- Classe/componente ideal: até 300 linhas.
- Arquivo ideal: até 500 linhas.
- Acima disso, dividir por responsabilidade antes de adicionar comportamento relevante.
- Nunca aceitar arquivo monolítico como solução final.

## Dívida técnica e arquivos grandes

- Se encontrar arquivo acima do limite, não refatorar tudo automaticamente.
- Antes de expandir arquivo grande, avaliar se a mudança pode ser extraída para service, adapter, component, mapper, policy ou helper coeso.
- Se a quebra não couber na entrega, registrar dívida técnica no pacote, bug ou backlog.
- Dívida técnica precisa ter: sintoma, risco, arquivo, impacto e proposta incremental.
- Proibido usar dívida técnica como desculpa para adicionar mais responsabilidade em arquivo já problemático.

## Contrato de módulo

Todo módulo relevante deve ter contrato claro, documentado no `ESCOPO.md` ou na seção do pacote.

Contrato mínimo:

- Responsabilidade do módulo.
- Entradas.
- Saídas.
- Dependências permitidas.
- Dependências proibidas.
- Regras de negócio que pertencem ao módulo.
- Regras que não pertencem ao módulo.
- Testes obrigatórios.
- Riscos e rollback.

## Nomenclatura oficial do projeto

Este modelo não impõe nomes universais como `Service`, `UseCase`, `Repository`, `Adapter`, `DTO` ou `Controller`.

Cada projeto deve registrar sua nomenclatura oficial em `ESCOPO.md` após inspeção da stack e dos padrões existentes.

Regras:

- Usar a nomenclatura já existente no projeto quando ela for consistente.
- Não misturar `Controller`, `Handler`, `Action` e `Endpoint` para o mesmo papel sem decisão registrada.
- Não misturar `Service`, `UseCase`, `Action` e `Interactor` para o mesmo papel sem decisão registrada.
- Não trocar nomenclatura só por preferência da IA.
- Se for necessário mudar nomenclatura, registrar a decisão em `DECISOES.md`.
- Consistência dentro do projeto é mais importante que preferência genérica do modelo.

Conceitos mínimos que o projeto deve nomear:

- entrada HTTP ou equivalente;
- caso de uso/regra de aplicação;
- regra de domínio;
- persistência;
- integração externa;
- payload de entrada;
- payload de saída;
- validação;
- regra variável;
- modelo de domínio;
- modelo de tela.

## Decisões de arquitetura

Decisão técnica relevante deve ser registrada em `DECISOES.md` para a IA não rediscutir o mesmo ponto.

Registrar quando houver:

- escolha de stack, biblioteca, padrão ou provider;
- mudança de boundary;
- criação de abstração;
- decisão de não refatorar agora;
- exceção a um padrão;
- trade-off de performance, prazo, risco ou compatibilidade.

O formato oficial fica em `DECISOES.md`.

## Workflow obrigatório

1. Inspecionar código, config, testes e docs antes de editar.
2. Confirmar branch atual antes de editar.
3. Se a branch for `main` ou `hml` e o usuário não tiver informado explicitamente que quer trabalhar nela, perguntar antes de continuar.
4. Manter mudanças pequenas e reversíveis.
5. Respeitar boundaries do módulo alterado.
6. Separar domínio, aplicação, infraestrutura e apresentação.
7. Validar entrada antes de persistência, upload, fila, cache, integração externa ou ação crítica.
8. Transformar explicitamente DTO, entidade, modelo local e modelo de UI.
9. Quando alterar contrato, ajustar consumidores no mesmo ciclo ou documentar compatibilidade temporária.
10. Se alterar UI, atualizar `TELAS.md`.
11. Se corrigir bug, retestar conforme `TESTES.md` e atualizar `BUGS.md`.
12. Se tomar decisão arquitetural relevante, registrar em `DECISOES.md`.
13. Fechar lote com validação rasa e focada.
14. Fechar pacote somente com validação completa, `./check.sh` verde e commit.
15. Mudança simples fora de pacote deve usar validação local proporcional, sem transformar tudo em pacote artificial.

## Playbook CRUD

CRUD não é uma tela única. Deve ser separado por responsabilidade.

### Telas esperadas

- Listagem e filtros: busca, filtros, paginação, ordenação, estados e ações de linha.
- Detalhamento: leitura completa, histórico, metadados, ações permitidas e contexto.
- Cadastro: criação, validação, submissão, sucesso e erro.
- Edição: alteração de registro existente, carregamento inicial, validação, conflito e sucesso.
- Formulário compartilhado: permitido para cadastro e edição, desde que não contenha regra de navegação, persistência ou carregamento remoto.

### Backend/API

1. Definir contrato público.
2. Definir validação de entrada.
3. Criar caso de uso/service por ação relevante.
4. Criar repository/adapter para persistência.
5. Criar handler/controller fino.
6. Padronizar resposta e erro.
7. Testar service e contrato de endpoint.

### Frontend/App

1. Criar camada de API/service.
2. Criar types/modelos de tela.
3. Criar tela de listagem.
4. Criar tela de detalhe.
5. Criar tela de cadastro.
6. Criar tela de edição.
7. Extrair formulário compartilhado sem regra de rota.
8. Cobrir loading, empty, error, success e retry.
9. Atualizar `TELAS.md`.

### Regras de CRUD

- Listagem não deve carregar detalhe completo de todos os itens.
- Detalhe não deve duplicar regra de edição.
- Cadastro e edição podem compartilhar formulário, mas submit e carregamento ficam fora do formulário.
- Delete deve ser soft delete salvo exceção documentada.
- Toda listagem deve ter estratégia explícita de paginação.
- Toda ação destrutiva deve ter confirmação e rollback funcional quando aplicável.

## Templates mínimos por tipo de entrega

Os templates abaixo são referência conceitual. Devem ser adaptados aos nomes e convenções reais da linguagem/framework do projeto.

### Backend/API

```txt
domain/
  entity ou model
  policy ou validator
application/
  use case ou service
infrastructure/
  repository ou adapter
transport/
  handler ou controller
  request/response DTO
tests/
  unit
  service/use case
  contract/API
```

### Web

```txt
feature/
  api ou service
  types ou schema
  pages
    list
    detail
    create
    edit
  components
    form
    filters
    table ou cards
  tests
```

### App

```txt
feature/
  data
  domain
  presentation
    controller ou viewmodel
    pages
    widgets
  tests
```

### Testes

```txt
unit: regra pura
service: fluxo de negócio
contract: API/payload
component/widget: UI crítica
e2e: fluxo principal ou regressão crítica
```

## Playbook tela web

1. Página coordena fluxo, rota, carregamento e permissões.
2. Componente apresenta UI.
3. Service encapsula HTTP.
4. Type/schema define contrato.
5. Formulário valida localmente antes de submeter.
6. Estados obrigatórios: loading, empty, error, success.
7. UI não chama banco, storage ou API espalhada em componente visual.
8. Texto final de produto, sem `PKG-XX`, debug ou nome interno.
9. Responsivo sem overflow.
10. Testar função pura, componente crítico e fluxo principal.

## Playbook tela app

1. Tela coordena estado e navegação.
2. Widget apresenta UI.
3. Controller/viewmodel concentra regra de exibição.
4. Repository encapsula remoto/local.
5. DTO não vaza como modelo visual quando há regra.
6. Estados explícitos: loading, empty, error, success, offline quando aplicável.
7. Validar responsividade, texto escalado e toque.
8. Atualizar `TELAS.md`.
9. Testar controller/viewmodel e widget principal.

## Playbook API

1. Handler/controller valida borda e traduz resposta.
2. Service/use case executa regra de negócio.
3. Repository/adapter executa persistência ou integração.
4. DTO/request/response isola contrato público.
5. Erros são padronizados.
6. Logs críticos têm contexto mínimo sem segredo.
7. Timeout, retry e idempotência quando houver integração.
8. Testes cobrem caso feliz, erro de validação, permissão e falha de dependência.

## Playbook SQL

1. Nunca usar migrations.
2. Criar `.sql` somente quando o projeto tiver banco.
3. Preferir scripts idempotentes.
4. Incluir cabeçalho com objetivo, ordem, impacto, rollback e validação.
5. Não usar `PKG-XX` em banco, eventos ou payload público.
6. Backfill e cleanup devem ser separados quando houver risco.

## Observabilidade mínima da aplicação

Observabilidade aqui é para a aplicação final em uso, não para log de desenvolvimento.

Fluxos críticos devem ser diagnosticáveis sem expor dados sensíveis.

Aplicável principalmente a:

- login/autorização;
- pagamento;
- dados sensíveis;
- mudança de status;
- exclusão;
- jobs;
- integrações externas;
- publicação/deploy;
- ações administrativas;
- banco;
- operação principal do produto.

Não é obrigatório para mudança simples de label, copy, espaçamento ou documentação.

### Logs técnicos

Registrar quando houver:

- erro de fluxo crítico;
- falha de integração externa;
- falha de autenticação/autorização;
- mudança de estado importante;
- job iniciado, finalizado ou com erro;
- deploy/publicação quando aplicável.

Campos recomendados:

- evento;
- nível;
- `request_id`, `correlation_id`, `job_id` ou equivalente;
- `user_id` quando existir;
- `tenant_id` ou `cliente_id` quando existir;
- entidade afetada quando existir;
- resultado;
- erro resumido.

Nunca registrar:

- senha;
- token;
- chave;
- segredo;
- payload sensível completo;
- dado pessoal desnecessário.

### Níveis de log

- `debug`: desenvolvimento e diagnóstico local.
- `info`: evento operacional esperado.
- `warning`: comportamento inesperado, mas recuperável.
- `error`: falha que impediu operação.
- `critical`: falha que afeta segurança, dados ou operação principal.

### Auditoria

Auditoria é para ação relevante de negócio, não para qualquer log técnico.

Registrar auditoria quando houver:

- alteração de permissão;
- ação administrativa;
- exclusão;
- mudança de status crítica;
- alteração de dado sensível;
- publicação ou operação irreversível.

Auditoria deve registrar:

- quem fez;
- quando;
- o que mudou;
- entidade afetada;
- origem;
- resultado.

### Banco e retenção

- Preferir logs estruturados, arquivos, serviço de log ou tabela existente antes de criar tabela nova.
- Não criar tabela nova de observabilidade sem justificativa em `DECISOES.md`.
- Toda persistência de log/auditoria precisa ter política de retenção e limpeza.
- Logs técnicos devem ter retenção curta por padrão.
- Auditoria pode ter retenção maior, conforme risco e requisito do produto.
- Se criar tabela, criar também estratégia de cleanup, job ou rotina operacional.
- Não guardar payload sensível completo para facilitar diagnóstico.

### Métricas mínimas

Quando viável, fluxo crítico deve permitir medir:

- sucesso/falha;
- tempo de resposta ou execução;
- quantidade de erro;
- falha de integração;
- status de job.

## Playbook testes

1. Por lote, rodar somente teste raso e focado no que mudou.
2. No fechamento do pacote, rodar validação completa dos módulos tocados.
3. Em mudança só documental, revisar diff e rodar check documental quando aplicável.
4. Em melhoria simples com código, rodar teste local/focado.
5. Em bug simples, reproduzir/retestar o fluxo específico.
6. Em bug complexo, rodar testes focados durante a correção e validação completa do fluxo afetado.
7. Teste unitário para regra pura.
8. Teste de service/use case para fluxo de negócio.
9. Teste de repository/adapter quando houver transformação ou persistência relevante.
10. Teste de contrato para API.
11. Teste de componente/widget para UI crítica.
12. E2E somente no fechamento do pacote, bug complexo ou regressão crítica.
13. Aplicar cobertura mínima por criticidade definida em `TESTES.md`.
14. Usar dados de teste e fixtures conforme política de `TESTES.md`.
15. Em fluxo crítico, validar observabilidade mínima quando aplicável.

## Review de fechamento de pacote

Antes do commit de fechamento, a IA deve revisar o pacote completo.

Checklist obrigatório:

1. Escopo do pacote foi respeitado.
2. Todos os lotes estão concluídos ou possuem bloqueio real documentado.
3. Mudanças fora do pacote foram removidas, separadas ou justificadas.
4. SOLID e boundaries reais foram respeitados.
5. Não há regra de negócio em UI, handler fino ou camada indevida.
6. Arquivos grandes não foram criados ou agravados sem plano incremental.
7. Contratos alterados foram documentados e consumidores ajustados.
8. Testes por criticidade foram executados.
9. Bugs relacionados foram retestados.
10. Regressões dos lotes anteriores foram verificadas.
11. `TELAS.md`, `TESTES.md`, `BUGS.md`, `RUNBOOK.md` e `DECISOES.md` foram atualizados quando aplicável.
12. SQL tem ordem, impacto, validação e rollback quando houver banco.
13. Branch foi conferida.
14. Diff final foi revisado.
15. `./check.sh` passou.
16. Commit de fechamento foi criado sem misturar outro pacote.

## Check adaptável por stack

Cada projeto deve adaptar `check.sh` para a própria linguagem, mantendo o contrato mínimo.

O `check.sh` deve orquestrar scripts pequenos em `scripts/`. Cada script valida uma regra específica. Ao copiar este modelo para um projeto real, ajustar `PATHS.toml` e os scripts para a stack e estrutura reais.

### Checks universais

- Arquivos obrigatórios existem.
- `PATHS.toml` aponta para arquivos reais.
- Markdown íntegro.
- Segredos óbvios bloqueados.
- `PKG-XX` não aparece em runtime.
- Arquivos grandes são detectados ou reportados.
- Cobertura percentual opcional pode ser ativada em `PATHS.toml`.
- Fixtures podem ser validadas quando configuradas em `PATHS.toml`.

### Scripts recomendados

- `scripts/validate-required-files.sh`
- `scripts/validate-paths.sh`
- `scripts/validate-docs.sh`
- `scripts/validate-rules.sh`
- `scripts/validate-no-secrets.sh`
- `scripts/validate-file-size.sh`
- `scripts/validate-no-runtime-pkg-names.sh`
- `scripts/validate-layering.sh`
- `scripts/validate-stack.sh`

### Exemplos por stack

- Go: `gofmt`, `go test`, `go vet`, build.
- Python: format/lint configurado, type-check quando houver, testes.
- PHP/Yii/Laravel: lint, static analysis, testes.
- Node/TypeScript: lint, type-check, testes, build.
- Flutter/Dart: `dart format`, `flutter analyze`, `flutter test`.
- Mobile nativo: lint, testes unitários, build/smoke quando aplicável.

### Adaptação obrigatória por projeto

- Preencher `quality.runtime_dirs` em `PATHS.toml`.
- Preencher comandos em `quality.stack` quando houver stack executável.
- Ajustar exclusões de diretórios gerados.
- Ajustar regra de arquivo grande para a linguagem.
- Ajustar regra de arquitetura por camada quando o projeto tiver estrutura consolidada.
- Preencher `quality.fixtures` quando o projeto usar seeds, fixtures ou E2E.
- Ativar `quality.layering` quando houver regra de imports/camadas verificável.

## Resposta final curta

Para reduzir tokens, a resposta final deve ser objetiva.

Formato recomendado:

```txt
Feito:
- ...

Validação:
- ...

Bloqueios:
- nenhum
```

Adicionar detalhes longos só quando houver risco, falha, decisão arquitetural, SQL, deploy, segurança ou pedido explícito.

## Contrato do check

`./check.sh` deve validar no mínimo:

- arquivos obrigatórios;
- coerência de `PATHS.toml`;
- integridade Markdown;
- ausência de indícios simples de segredo;
- checks de linguagem/stack configurados no projeto real;
- regra contra arquivos grandes quando viável;
- regra contra `PKG-XX` em runtime quando viável.

## Branch, commit e PR

1. Usar branch definida pelo projeto. Se não houver regra, usar `codex/` ou `mayder/`.
2. Se a branch atual for `main` ou `hml` e o usuário não tiver autorizado explicitamente o uso dela, perguntar antes de editar, commitar ou rodar fluxo de entrega.
3. Um commit por pacote fechado, salvo correção emergencial ou pedido explícito.
4. Mensagem de commit em modo imperativo.
5. Corpo do commit deve citar `Check: ./check.sh`.
6. Se houver `.sql`, citar scripts e ordem de execução.
7. PR deve trazer contexto, resumo, escopo, checks, testes, riscos e rollback.
8. Push não é obrigatório no fechamento local do pacote.
9. Mudança simples fora de pacote só exige commit quando solicitado ou quando for uma mudança lógica pronta para preservar.

## Definition of Done

1. Escopo respeitado.
2. SOLID aplicado de forma verificável.
3. Boundaries respeitados.
4. Arquivos não cresceram além do razoável sem divisão.
5. Entradas validadas.
6. Erros tratados com mensagem objetiva.
7. Contratos documentados quando mudaram.
8. Testes automatizados ou validação manual registrados.
9. `DEMANDAS.md`, `TELAS.md`, `TESTES.md` e `BUGS.md` sincronizados quando aplicável.
10. Validação rasa registrada por lote.
11. Validação completa executada no fechamento do pacote.
12. Bug ou melhoria fora de pacote validado proporcionalmente ao risco.
13. Review de fechamento de pacote realizado.
14. `./check.sh` passou quando aplicável ou bloqueio foi documentado.
15. Commit de fechamento do pacote realizado.

## Adaptacao deste projeto

- Projeto: `aiceberg_agent_poc`
- Tipo: POC Go
- Stack detectada: Go POC, gopsutil
- Regra: adaptar comandos, camadas, testes, fixtures e limites conforme a realidade deste modulo Go.
- Go: validar `gofmt`, `go test ./...`, `go vet ./...` e build quando houver `cmd/*`.

## Regras essenciais do modelo

- SOLID e separacao de responsabilidades sao obrigatorios para qualquer implementacao.
- Leitura mínima por tipo de tarefa: seguir `PATHS.toml` e abrir somente os arquivos necessarios para pacote, bug, UI, arquitetura ou docs.
- Resposta final curta: informar o que foi feito, bloqueios e como validar.
- Se a branch atual for `main` ou `hml`, confirmar com o usuario antes de alterar, exceto quando ja houver autorizacao explicita para o lote.
- Nunca usar migrations. Mudancas de banco devem ser scripts `.sql`, preferencialmente idempotentes, com ordem e rollback.
- Observabilidade mínima da aplicação: logs estruturados, correlacao quando aplicavel, metricas/auditoria proporcionais ao risco e sem criar tabelas desnecessarias.
- Toda observabilidade persistida precisa de retenção e limpeza documentadas.
- Review de fechamento de pacote: revisar bugs, regressao, arquitetura, testes, docs, riscos e rollback antes de encerrar.
- Contrato de módulo: cada modulo deve declarar responsabilidade, entrada, saida, erros, dependencias e limites de camada.
- Nomenclatura oficial do projeto: registrar nomes aprovados para handler, service, use case, repository, adapter, DTO/schema e evitar sinonimos sem decisao.
- Adaptação à arquitetura real: ao aplicar este modelo em projeto real, a IA deve inspecionar linguagem, framework, pastas, comandos, testes e convencoes antes de editar.
- Decisoes arquiteturais devem ser registradas em `DECISOES.md` usando o formato `DEC-YYYYMMDD-01`.
- Check adaptável por stack: `check.sh` deve chamar validacoes do modelo e os comandos reais da linguagem/framework do projeto.
