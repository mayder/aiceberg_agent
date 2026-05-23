# TELAS.md

Inventário operacional de telas, rotas, estados e regras de UI.

## Regras

- Modal, drawer, bottom sheet ou painel lateral conta como tela quando exige interação relevante.
- Estados loading, empty, error, success, offline e parcial devem ser descritos quando existirem.
- Toda tela alterada deve ser revisada antes de concluir entrega.
- Este arquivo descreve comportamento final de produto, não implementação interna.
- CRUD deve separar listagem/filtros, detalhe, cadastro e edição.

## Padrão para CRUD

### Listagem e filtros

- Objetivo: encontrar, comparar e acessar registros.
- Deve conter busca/filtros, paginação, ordenação, estado vazio, erro, loading e ações permitidas.
- Não deve carregar detalhe completo de todos os registros.

### Detalhamento

- Objetivo: ler o registro completo e executar ações contextuais.
- Deve conter metadados, histórico quando existir, permissões e ações claras.
- Não deve misturar formulário de edição completo salvo decisão documentada.

### Cadastro

- Objetivo: criar registro novo.
- Deve conter formulário, validação, sucesso, erro e cancelamento.
- Não deve depender de dados de registro existente.

### Edição

- Objetivo: alterar registro existente.
- Deve conter carregamento inicial, formulário, validação, conflito, sucesso, erro e cancelamento.
- Pode reutilizar formulário de cadastro, mas carregamento e submissão ficam fora do formulário compartilhado.

## Organização por telas

- `TELA-01` Exemplo de listagem e filtros
- `TELA-02` Exemplo de detalhamento
- `TELA-03` Exemplo de cadastro
- `TELA-04` Exemplo de edição

## [TELA-01] Exemplo de listagem e filtros

Status atual: modelo.

- Rota/entrada:
- Objetivo:
- Origem:
- Permissões:
- Filtros:
- Colunas/cards:
- Ações de linha:
- Ação principal:
- Estados esperados: loading, empty, error, success, retry.
- Regras de exibição:
- Paginação/ordenação:
- Regressões próximas:
- Pacotes relacionados:
- Bugs relacionados:

## [TELA-02] Exemplo de detalhamento

Status atual: modelo.

- Rota/entrada:
- Objetivo:
- Origem:
- Permissões:
- Campos e informações:
- Ações permitidas:
- Histórico/metadados:
- Estados esperados: loading, not found, error, success, retry.
- Regras de exibição:
- Regressões próximas:
- Pacotes relacionados:
- Bugs relacionados:

## [TELA-03] Exemplo de cadastro

Status atual: modelo.

- Rota/entrada:
- Objetivo:
- Origem:
- Permissões:
- Campos:
- Validações:
- Ações:
- Estados esperados: initial, validating, submitting, success, error.
- Regras de exibição:
- Regressões próximas:
- Pacotes relacionados:
- Bugs relacionados:

## [TELA-04] Exemplo de edição

Status atual: modelo.

- Rota/entrada:
- Objetivo:
- Origem:
- Permissões:
- Carregamento inicial:
- Campos:
- Validações:
- Ações:
- Estados esperados: loading, not found, validating, submitting, success, error, conflict.
- Regras de exibição:
- Regressões próximas:
- Pacotes relacionados:
- Bugs relacionados:

## Adaptacao deste projeto

- Projeto: `aiceberg_agent_poc`
- Tipo: POC Go
- Stack detectada: Go POC, gopsutil
- Regra: adaptar comandos, camadas, testes, fixtures e limites conforme a realidade deste modulo Go.

## CRUD e telas

- Listagem e filtros devem ficar em tela propria.
- Detalhamento deve ficar em tela propria.
- Cadastro deve ficar separado de listagem e detalhamento.
- Edicao deve ficar separada de listagem e detalhamento.
- Cadastro e edicao podem compartilhar formulario, desde que a responsabilidade do formulario seja isolada.
