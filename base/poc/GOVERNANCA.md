# GOVERNANCA.md

Fonte de verdade para prontidão de release, prioridade, métricas, dependências, riscos e rollback.

## Escopo

Este arquivo governa decisões transversais. Backlog fica em `DEMANDAS.md`; workflow, arquitetura e SOLID ficam em `QUALITY_ROADMAP.md`.

## Gates de release

Nenhuma entrega deve ser considerada pronta sem cumprir os gates aplicáveis.

### Qualidade

- `./check.sh` verde.
- Branch `main` ou `hml` só deve ser usada sem pergunta quando o usuário autorizar explicitamente.
- Sem bug crítico ou alto aberto na frente alterada.
- Fluxo principal validado com evidência objetiva.
- Cobertura mínima por criticidade respeitada conforme `TESTES.md`.
- Validação rasa registrada em cada lote.
- Validação completa executada no fechamento do pacote.
- Review de fechamento executado antes do commit do pacote.
- Bugs e melhorias fora de pacote validados proporcionalmente ao risco.
- Contratos alterados documentados no mesmo ciclo.
- SOLID e boundaries respeitados.
- Nenhum arquivo monolítico novo ou agravado sem justificativa e plano de quebra.
- Commit realizado para pacote 100% fechado.

### Segurança

- Segredos fora do repositório.
- Entrada validada antes de persistência, upload, ação crítica ou integração externa.
- Autorização centralizada por papel/recurso quando aplicável.
- Logs sem tokens, senhas, chaves ou payload sensível completo.
- Dados sensíveis mascarados em erro, log, print e relatório.

### Operação

- Mudança crítica com plano de rollback.
- Jobs, filas e integrações com timeout, retry e idempotência quando aplicável.
- Fluxos críticos com observabilidade mínima proporcional ao risco.
- Persistência de log/auditoria com retenção e limpeza.
- Nova tabela de observabilidade só com justificativa e política de cleanup.
- Checks dependentes de infraestrutura externa devem ser opt-in com skip explícito.
- Deploy/publicação com smoke test mínimo.
- `RUNBOOK.md` atualizado quando muda forma de operar, publicar ou reverter.

### Produto/UI

- Textos finais de produto, sem nomes internos de pacote, debug ou implementação.
- Mudança visual grande separada de mudança de dados/contrato quando possível.
- CRUD separado em listagem/filtros, detalhe, cadastro e edição.
- Evidência visual para UI alterada quando houver navegador, emulador ou device disponível.

## Estratégia de testes por pacote

- Lote em andamento: teste raso, rápido e focado.
- Pacote 100% implementado: teste completo e regressão dos lotes anteriores.
- Falha encontrada no fechamento deve ser corrigida antes do commit, mesmo que tenha origem em lote anterior.
- Review de fechamento deve cobrir escopo, regressões, arquitetura, testes, docs, SQL, branch e diff final.
- Commit é obrigatório no fechamento do pacote; push não é obrigatório.

## Estratégia para bug e melhoria simples

- Mudança só documental: revisão de diff e check documental quando aplicável.
- Melhoria simples com código local: teste local/focado e validação visual quando viável.
- Bug simples: reprodução/reteste focado.
- Bug complexo: teste automatizado quando viável, `./check.sh` e validação completa do fluxo afetado.
- Se uma melhoria simples crescer em escopo, criar pacote ou vincular a pacote existente.

## Priorização oficial

- P0: segurança, login, dados, operação mínima, pagamento, integridade, release bloqueada e arquitetura que inviabiliza manutenção.
- P1: fluxos principais, usabilidade, testes, observabilidade e redução de retrabalho.
- P2: otimizações, automações e melhorias incrementais.

## Banco de dados

- Migrations são proibidas.
- Toda alteração de banco deve vir como `.sql`.
- Preferir scripts idempotentes.
- Incluir ordem de execução, validação e rollback.
- Mudança irreversível exige confirmação explícita.

## Riscos e rollback

| Área | Risco | Rollback esperado |
|---|---|---|
| Arquitetura | arquivo monolítico ou regra espalhada | quebrar por responsabilidade antes de expandir |
| Contrato | consumidor quebrar por payload incompatível | compatibilidade temporária ou revert coordenado |
| Banco | script com impacto indevido | script corretivo ou restauração documentada |
| UI | regressão visual ou fluxo confuso | reverter componente/tela ou esconder por flag |
| Segurança | vazamento ou autorização incorreta | bloquear rota/ação, reverter e auditar |
| Operação | deploy parcial ou serviço instável | voltar versão anterior e executar smoke |
| Git | vários pacotes misturados no mesmo commit | fechar e commitar um pacote por vez |
| Branch | editar sem querer em `main` ou `hml` | perguntar antes de continuar, salvo autorização explícita |
| Review | pacote fechado com escopo misturado ou docs atrasadas | review obrigatório antes do commit |
| Observabilidade | tabelas demais ou logs sem limpeza | reutilizar estrutura existente, definir retenção e cleanup |

## Métricas mínimas

- Check local reproduzível.
- Fluxos principais com validação documentada.
- Bugs críticos/altos tratados antes de release.
- Rollback claro para mudanças de maior risco.
- Testes cobrindo regra de negócio central.
- Telas críticas documentadas em `TELAS.md`.
- Commit por pacote fechado.
- P0 com evidência de regra, contrato, permissão/erro quando aplicável e fluxo principal.
- Review de fechamento realizado antes de commit.
- Fluxo crítico diagnosticável sem expor dado sensível.

## Relação com outros arquivos

- `PATHS.toml`: paths e checks autoritativos.
- `QUALITY_ROADMAP.md`: workflow, SOLID, arquitetura e Definition of Done.
- `DEMANDAS.md`: backlog executável.
- `TESTES.md`: testes e validação.
- `TELAS.md`: inventário operacional de UI.
- `BUGS.md`: defeitos e retestes.

## Adaptacao deste projeto

- Projeto: `aiceberg_agent_poc`
- Tipo: POC Go
- Stack detectada: Go POC, gopsutil
- Regra: adaptar comandos, camadas, testes, fixtures e limites conforme a realidade deste modulo Go.

## Banco e governanca tecnica

- Migrations são proibidas. Alteracoes de banco devem ser entregues como scripts `.sql`, preferencialmente idempotentes.
- Mudancas com risco transversal exigem rollback, evidencia de teste e registro em `DECISOES.md` quando criarem padrao duradouro.
