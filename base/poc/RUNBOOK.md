# RUNBOOK.md

Runbook operacional do projeto.

## Objetivo

Padronizar como rodar, diagnosticar, publicar, validar e reverter o projeto.

## Atualização deste modelo em projeto novo

1. Copiar arquivos para a raiz do projeto.
2. Ajustar `PATHS.toml`.
3. Inspecionar linguagem, framework, estrutura de pastas e convenções existentes.
4. Preencher `ESCOPO.md` com produto, módulos, stack e arquitetura real.
5. Preencher a nomenclatura oficial do projeto em `ESCOPO.md`.
6. Ajustar `QUALITY_ROADMAP.md` para stack real, mantendo SOLID.
7. Ajustar `GOVERNANCA.md` para riscos reais.
8. Trocar exemplos de `DEMANDAS.md` por pacotes reais.
9. Mapear telas reais em `TELAS.md`.
10. Definir testes reais em `TESTES.md`.
11. Registrar decisões iniciais relevantes em `DECISOES.md`.
12. Rodar `./check.sh`.

## Checklist de aplicação em projeto real

- [ ] Confirmar branch e status.
- [ ] Ler `PATHS.toml` antigo se existir.
- [ ] Inspecionar stack, framework e comandos disponíveis.
- [ ] Mapear diretórios runtime reais.
- [ ] Mapear testes existentes.
- [ ] Mapear fixtures/seeds existentes.
- [ ] Mapear estrutura de camadas e imports proibidos.
- [ ] Preencher arquitetura real em `ESCOPO.md`.
- [ ] Preencher nomenclatura oficial.
- [ ] Ajustar `quality.runtime_dirs`.
- [ ] Ajustar `quality.stack`.
- [ ] Ajustar `quality.fixtures`.
- [ ] Ativar `quality.layering` se houver regra verificável.
- [ ] Registrar decisões iniciais em `DECISOES.md`.
- [ ] Rodar `./check.sh`.
- [ ] Corrigir scripts até o check representar a realidade do projeto.

## Adaptação do check por projeto

1. Preencher `quality.runtime_dirs` em `PATHS.toml` com diretórios de código real.
2. Preencher `quality.stack` com comandos da linguagem.
3. Preencher `quality.fixtures` quando houver fixtures, seeds, contrato ou E2E.
4. Ajustar `scripts/validate-file-size.sh` se a stack exigir exceções.
5. Ajustar `scripts/validate-no-runtime-pkg-names.sh` para ignorar fixtures ou docs embutidas.
6. Configurar `scripts/validate-layering.sh` quando o projeto tiver estrutura definida.
7. Manter `check.sh` como orquestrador; evitar colocar todas as regras diretamente nele.
8. Não inventar estrutura nova antes de mapear a arquitetura atual.

## Ambientes

| Ambiente | URL/host | Observações |
|---|---|---|
| Local |  |  |
| Homologação |  |  |
| Produção |  |  |

## Comandos locais

```bash
./check.sh
```

## Variáveis e segredos

- Segredos ficam fora do Git.
- `.env.example` documenta chaves sem valores reais quando existir.
- Nunca registrar token, senha ou chave em log, docs, print ou commit.

## Deploy/publicação

1. Confirmar branch e diff.
2. Rodar `./check.sh`.
3. Confirmar scripts SQL necessários.
4. Executar deploy conforme stack do projeto.
5. Rodar smoke test.
6. Monitorar logs.

## Smoke test mínimo

- Health/readiness ou página principal responde.
- Login ou fluxo principal abre.
- Ação principal do pacote funciona.
- Logs não exibem erro crítico.

## Diagnóstico e observabilidade

Para fluxos críticos, o runbook deve explicar:

- onde ver logs;
- como filtrar por `request_id`, `correlation_id`, `job_id` ou equivalente;
- onde ver auditoria quando existir;
- quais métricas indicam falha;
- como diferenciar erro de usuário, erro de integração e erro interno;
- política de retenção;
- como funciona a limpeza automática.

Regra operacional:

- não criar tabela nova de log/auditoria se estrutura existente resolver;
- se criar tabela, documentar retenção e cleanup;
- não guardar payload sensível completo.

## Rollback

1. Identificar versão anterior estável.
2. Reverter deploy/código.
3. Executar rollback SQL somente quando documentado e necessário.
4. Rodar smoke test.
5. Registrar causa e ação tomada.

## Incidentes

Modelo:

```txt
Data/hora:
Ambiente:
Sintoma:
Impacto:
Causa provável:
Ação imediata:
Rollback:
Validação:
Próximo ajuste:
```

## Adaptacao deste projeto

- Projeto: `aiceberg_agent_poc`
- Tipo: POC Go
- Stack detectada: Go POC, gopsutil
- Regra: adaptar comandos, camadas, testes, fixtures e limites conforme a realidade deste modulo Go.
- Banco de dados: nao usar migrations. Mudancas devem ser scripts `.sql` idempotentes quando possivel, com ordem e rollback.
