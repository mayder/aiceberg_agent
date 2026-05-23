# Modelo de projetos

Modelo universal para orientar desenvolvimento com IA, reduzindo ambiguidade, tokens e retrabalho.

## Objetivo

Garantir que qualquer projeto comece com padrões claros de:

- SOLID;
- separação de responsabilidades;
- backlog executável;
- CRUD bem dividido;
- testes;
- bugs e reteste;
- operação e rollback.

## Arquivos principais

- `PATHS.toml`: fonte de verdade para paths, módulos, checks e ordem de leitura.
- `ESCOPO.md`: produto, módulos, stack, fora de escopo e princípios inegociáveis.
- `GOVERNANCA.md`: gates de release, prioridade, risco e rollback.
- `QUALITY_ROADMAP.md`: SOLID, workflow, playbooks e Definition of Done.
- `DEMANDAS.md`: backlog executável por pacotes e lotes.
- `BUGS.md`: registro de bugs, diagnóstico, correção e reteste.
- `TELAS.md`: inventário operacional de telas e estados.
- `TESTES.md`: testes automatizados, validação manual e reteste.
- `DECISOES.md`: decisões arquiteturais, técnicas e operacionais relevantes.
- `RUNBOOK.md`: operação, deploy, smoke e rollback.
- `check.sh`: validação local mínima.
- `scripts/`: validadores pequenos chamados pelo `check.sh`.
- `MAPA_EXECUTIVO_MARKMAP.md`: mapa mental curto para visão rápida.
- `MAPA_MENTAL_MARKMAP.md`: mapa mental completo para referência detalhada.

## Como usar em um projeto novo

1. Copiar estes arquivos para a raiz do projeto.
2. Ajustar `PATHS.toml` com paths reais.
3. Inspecionar linguagem, framework, arquitetura e estrutura existente.
4. Preencher `ESCOPO.md` com arquitetura real e boundaries.
5. Preencher a nomenclatura oficial do projeto em `ESCOPO.md`.
6. Ajustar `QUALITY_ROADMAP.md` para a stack real, sem remover SOLID.
7. Ajustar `GOVERNANCA.md` com riscos reais.
8. Criar pacotes reais em `DEMANDAS.md`.
9. Mapear telas reais em `TELAS.md`.
10. Definir testes reais em `TESTES.md`.
11. Ajustar `quality.runtime_dirs` e `quality.stack` em `PATHS.toml`.
12. Adaptar scripts de validação para a stack real.
13. Rodar `./check.sh`.

## Regra para IA

Não ler tudo por padrão. Ler `PATHS.toml`, identificar tipo de tarefa e então ler apenas o necessário.

Não inventar estrutura nova antes de inspecionar a arquitetura real do projeto.

Responder de forma curta no fechamento: feito, validação, bloqueios. Detalhar só quando houver risco, falha, SQL, decisão, deploy ou pedido explícito.

## Validação

```bash
./check.sh
```

## Adaptacao deste projeto

- Projeto: `aiceberg_agent_poc`
- Tipo: POC Go
- Stack detectada: Go POC, gopsutil
- Regra: adaptar comandos, camadas, testes, fixtures e limites conforme a realidade deste modulo Go.
