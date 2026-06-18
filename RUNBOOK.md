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

## Matriz de paridade do agente

A matriz operacional fica no repo web em:

```txt
/Users/brenomayder/projects/web/public/aiceberg_web/docs/agente_datadog_paridade.md
```

O inventario tecnico local fica em:

```txt
docs/pkg58_inventario_agente_atual.md
```

Antes de implementar PKG-59 a PKG-72:

1. Confirmar dependencia na matriz.
2. Preservar contratos HTTP legados e `/v1/agent/channel`.
3. Adicionar payload novo de forma opcional ou versionada.
4. Criar flag/config de rollback antes de ativar coletor novo.
5. Validar agente instalado anteriormente.
6. Garantir que logs e flare nao exponham token, segredo, payload sensivel ou comando inseguro.

Nao declarar superioridade sobre Datadog sem benchmark, evidencia funcional e comparacao objetiva registrada na matriz.

### PKG-59 - Runtime Collector/Forwarder

Contrato tecnico: `docs/pkg59_runtime_architecture.md`.

Diagnostico local:

```bash
curl -s http://127.0.0.1:<HEALTH_PORT>/health
```

Campos esperados:

- `agent_pipeline_version`;
- `queue_items`;
- `queue_bytes`;
- `flush_detail`;
- `channel`, quando o canal estiver ativo.

Diagnostico remoto seguro:

- usar comando permitido `inspect_runtime_config`;
- conferir `scheduler_snapshot` e `agent_pipeline_version`;
- nao executar shell remoto ou script arbitrario.

Rollback: voltar o artefato do agente para a versao anterior e manter os campos aditivos ignorados pelo backend. O PKG-59 nao exige SQL.

### PKG-60 - Pipeline seguro de logs

Contrato tecnico: `docs/pkg60_logs_pipeline.md`.

Configuracoes locais:

- `OSLOG_ENABLED=true`;
- `OSLOG_FILES=/var/log/auth.log,/var/log/syslog`;
- `OSLOG_WIN_CHANNELS=Security,System,Application,Microsoft-Windows-Sysmon/Operational`;
- `OSLOG_INCLUDE_REGEX`;
- `OSLOG_EXCLUDE_REGEX`;
- `OSLOG_MIN_SEVERITY`.

Config remota equivalente em `logs.include_regex`, `logs.exclude_regex` e `logs.min_severity`.

Rollback: desligar logs por `OSLOG_ENABLED=false` ou remover flags remotas `OSLogFiles`/`OSLogWinChannels`. Se a regressao for no binario, publicar a versao anterior.

Nao registrar conteudo descartado por filtro; usar apenas `dropped_count`.

### PKG-61 - Metricas custom locais

Contrato tecnico: `docs/pkg61_custom_metrics.md`.

Configuracoes locais:

- `CUSTOM_METRICS_ENABLED=true`;
- `CUSTOM_METRICS_UDP_ADDR=127.0.0.1:8125`;
- `CUSTOM_METRICS_HTTP_ADDR=127.0.0.1:8126`;
- `CUSTOM_METRICS_INTERVAL=10`;
- `CUSTOM_METRICS_MAX_SERIES=1000`;
- `CUSTOM_METRICS_MAX_BYTES=65536`.

Rollback: definir `CUSTOM_METRICS_ENABLED=false` ou enviar config remota `custom_metrics.enabled=false`. Nao ha SQL do PKG-61.

### PKG-62 - OTLP HTTP/JSON

Contrato tecnico: `docs/pkg62_otlp_http_json.md`.

Configuracoes locais:

- `OTLP_ENABLED=true`;
- `OTLP_HTTP_ADDR=127.0.0.1:4318`;
- `OTLP_INTERVAL=10`;
- `OTLP_MAX_ITEMS=1000`;
- `OTLP_MAX_BYTES=1048576`.

Rollback: definir `OTLP_ENABLED=false` ou enviar config remota `otlp.enabled=false`. Nao ha SQL do PKG-62.

### PKG-64 - Containers Docker inicial

Contrato tecnico: `docs/pkg64_containers.md`.

Configuracoes locais:

- `CONTAINER_ENABLED=true`;
- `CONTAINER_DOCKER_SOCKET=/var/run/docker.sock`;
- `CONTAINER_INTERVAL=30`;
- `CONTAINER_MAX_ITEMS=200`.

Rollback: definir `CONTAINER_ENABLED=false` ou config remota `containers.enabled=false`. Nao ha SQL do PKG-64.

### PKG-65 - Kubernetes DaemonSet e Helm inicial

Contrato tecnico: `docs/pkg65_kubernetes.md`.

Instalacao:

- manifest direto: `deploy/kubernetes/aiceberg-agent.yaml`;
- Helm chart: `deploy/helm/aiceberg-agent`;
- secret obrigatorio: `aiceberg-agent/token`.

Configuracoes principais:

- `KUBERNETES_ENABLED=true`;
- `KUBERNETES_NODE_NAME` via `spec.nodeName`;
- `KUBERNETES_NAMESPACE` opcional;
- `KUBERNETES_INTERVAL=30`;
- `KUBERNETES_MAX_ITEMS=500`;
- `KUBERNETES_MAX_EVENTS=100`.

RBAC minimo: leitura de `nodes`, `pods` e `events`. Nao conceder `secrets`, `exec`, `pods/log`, `update`, `patch` ou `delete` sem nova decisao.

Rollback: `helm uninstall aiceberg-agent -n aiceberg`, `kubectl delete -f deploy/kubernetes/aiceberg-agent.yaml` ou config remota `kubernetes.enabled=false`. Nao ha SQL do PKG-65.

### PKG-66 - Runtime de checks locais

Contrato tecnico: `docs/pkg66_local_checks.md`.

Configuracoes locais:

- `LOCAL_CHECKS_ENABLED=true`;
- `LOCAL_CHECKS_INTERVAL=30`;
- `LOCAL_CHECKS_MAX_CHECKS=100`;
- `LOCAL_CHECKS_MAX_BYTES=1048576`;
- `LOCAL_CHECKS_JSON=[...]`.

Tipos permitidos: `http`, `tcp`, `openmetrics`, `jmx`, `postgresql`, `mysql`, `redis`, `nginx`, `apache`, `iis_wmi`, `windows_service`.

Rollback: definir `LOCAL_CHECKS_ENABLED=false` ou config remota `local_checks.enabled=false`. Nao ha SQL do PKG-66.

### PKG-67 - Fleet, rollout e flare seguro

Contrato tecnico: `docs/pkg67_fleet_rollout_flare.md`.

Diagnostico:

- `inspect_runtime_config` mostra `fleet_runtime`;
- `collect_support_flare` coleta evidencias sanitizadas;
- acompanhar update remoto por `/v1/agent/update-report`.

Politica operacional:

- rollout global exige canario;
- update precisa `version`, `url` e `sha256`;
- rollback usa artefato anterior conhecido e acompanha `version_confirmed`;
- flare nao pode conter token, secret, password, authorization ou cookie.

Rollback: desabilitar auto-update remoto e o comando `collect_support_flare` no backend. Nao ha SQL do PKG-67.

### PKG-68 - Seguranca e assinatura

Contrato tecnico: `docs/pkg68_security_hardening.md`.

Variaveis:

- `REMOTE_CONFIG_SIGNATURE_SECRET`;
- `REMOTE_CONFIG_SIGNATURE_REQUIRED=true`;
- `REMOTE_CONFIG_ALLOW_UNSIGNED_SENSITIVE=false`;
- `TLS_INSECURE_SKIP_VERIFY=false`;
- `TLS_INSECURE_ALLOW_PROD=false`.

Regras:

- payload sensivel sem assinatura deve ser rejeitado quando segredo de assinatura existir;
- downgrade de update sem `force=true` e bloqueado;
- `token_rotation.new_token` nunca deve aparecer em log;
- `security_runtime` deve indicar politica sem expor segredo;
- proxy corporativo segue `HTTP_PROXY`, `HTTPS_PROXY` e `NO_PROXY`.

Rollback: desativar obrigatoriedade de assinatura ou permitir unsigned sensitive apenas durante janela controlada. Nao ha SQL do PKG-68.

### PKG-69 - Matriz operacional real

Contrato tecnico: `docs/pkg69_operational_matrix.md`.

Comando local:

```bash
scripts/pkg69_operational_homologation.sh
```

O comando valida ambiente local, testes focados e `./check.sh`, e lista pendencias reais por Windows, Linux, Docker, Kubernetes, proxy, disco, payload e update rollback.

Rollback: nao altera runtime; se a validacao real falhar, reabrir pacote tecnico correspondente e manter artefato anterior.

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

- Projeto: `aiceberg_agent`
- Tipo: Agente Go
- Stack detectada: Go agent/CLI, gopsutil, NTP, SNMP
- Regra: adaptar comandos, camadas, testes, fixtures e limites conforme a realidade deste modulo Go.
- Banco de dados: nao usar migrations. Mudancas devem ser scripts `.sql` idempotentes quando possivel, com ordem e rollback.
