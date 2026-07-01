# BUGS.md

Registro oficial de bugs conhecidos, reprodução, impacto, correção e reteste.

## Regras

- Todo bug precisa ter severidade, área, passos de reprodução, esperado, observado e critério de fechamento.
- Bug corrigido só fecha após reteste seguindo `TESTES.md`.
- Bugs críticos ou altos bloqueiam release da frente afetada.
- Bugs de UI precisam de evidência visual quando tecnicamente possível.
- Não apagar bugs antigos; atualizar status e histórico.
- Causa provável deve separar hipótese principal de alternativas.

## Severidades

- Crítica: impede login, operação principal, pagamento, entrega, publicação ou expõe dados.
- Alta: quebra fluxo importante, causa interpretação operacional errada ou fere segurança.
- Média: degrada experiência sem bloquear fluxo principal.
- Baixa: ajuste visual, texto ou comportamento secundário.

## Diagnóstico obrigatório

1. Causa mais provável.
2. Uma ou duas hipóteses alternativas.
3. Menor correção possível.
4. Como validar.

## Validação por risco

### Bug simples

- Correção local e pequena.
- Reteste focado no passo de reprodução.
- Teste automatizado só é obrigatório se já existir cobertura próxima ou se a correção alterar regra.
- Não exige suíte completa.

### Bug complexo

- Toca regra, contrato, banco, permissão, segurança, integração, concorrência, sincronização ou vários módulos.
- Exige teste automatizado novo ou ajustado quando tecnicamente viável.
- Exige `./check.sh`.
- Exige validação completa do fluxo afetado e regressões próximas.

## Organização

## [BUG-20260701-01] Amostra de CPU Windows podia reportar 100% falso

Status: corrigido
Severidade: Alta
Área: Métricas do agente
Módulo: `internal/platform/collectors/sysmetrics`
Pacote relacionado: observabilidade operacional do agente
Tela relacionada: Detalhe do agente / alertas de threshold
Data: 2026-07-01

Reprodução:
- Agente Windows `0.8.37` coletando `sysmetrics`.
- Backend recebia `cpu.percent_total=100` enquanto `percent_per_cpu` vinha baixo ou em padrão binário `0/100`.
- Windows Task Manager/Resource Monitor mostravam uso real próximo de 1%.

Esperado:
- A CPU total deve representar a média real da janela de coleta.
- Amostra incoerente deve ser omitida para não abrir alerta falso.

Observado:
- `cpu.PercentWithContext(ctx, 0, false)` seguido de `cpu.PercentWithContext(ctx, 0, true)` gerava amostra instantânea incoerente no Windows.

Causa provável:
- Uso de intervalo `0` no gopsutil em duas leituras sequenciais de CPU no Windows.

Hipóteses alternativas:
- Saturação real de poucos núcleos deve continuar visível, mas não pode virar CPU total 100% sem média coerente.

Correção:
- Coleta de CPU passa a usar janela curta de amostragem.
- CPU total passa a ser derivada da média por núcleo quando a amostra por núcleo é coerente.
- Padrão binário `0/100` no Windows é descartado na amostra atual.
- Versão do agente elevada para `0.8.38`.

Critério de fechamento:
- `go test ./internal/platform/collectors/sysmetrics` passa.
- `./check.sh` passa.
- Artefatos `0.8.38` são gerados por `./scripts/build_installers.sh` e publicados no web.

## [BUG-20260624-02] Falhas recorrentes do worker eram reabertas indefinidamente

Status: corrigido
Severidade: Alta
Área: Observabilidade do agente
Módulo: `internal/bootstrap/app.go`
Pacote relacionado: observabilidade operacional
Tela relacionada: Detalhe do agente
Data: 2026-06-24

Reprodução:
- Agente com falha recorrente em oslogs, containers, kubernetes, flush, ping, config ou self-healing.
- A cada janela de coleta, o agente reportava `recovery_status=open` novamente.
- A web acumulava milhares de ocorrências abertas.

Esperado:
- Fingerprint estável por tipo/modo/contexto operacional.
- Falha repetida deve ter throttle maior.
- Sucesso posterior deve reportar `recovered` para fechar o estado aberto.

Observado:
- Fingerprint incluía texto do erro, que pode variar.
- Throttle era curto.
- Ciclo com sucesso não emitia recuperação para os tipos recorrentes.

Causa provável:
- O reportador local registrava falhas, mas não modelava estado de recuperação.

Hipóteses alternativas:
- Falhas reais de backend ou permissão devem continuar aparecendo, mas sem multiplicar indefinidamente o estado aberto.

Correção:
- Fingerprint usa `error_type`, modo e contexto estável (`route`, `source`, `name`, `command_code`).
- Repetição do mesmo estado fica limitada a 30 minutos para `open` e 5 minutos para `recovered`.
- Ciclos bem-sucedidos de containers, Kubernetes, flush, ping, config, oslogs e self-healing reportam `recovered`.

Critério de fechamento:
- `go test ./internal/bootstrap ./internal/data/remote` passa.
- O agente não infla indefinidamente o contador de ocorrências abertas para a mesma falha recorrente.

## [BUG-20260620-01] Coleta vazia de oslogs era tratada como erro e podia virar log bruto

Status: corrigido
Severidade: Alta
Área: Coleta de logs
Módulo: `internal/platform/collectors/oslogs`
Pacote relacionado: PKG-60, PKG-73, PKG-74
Tela relacionada: Detalhe de log no web
Data: 2026-06-20

Reprodução:
- Habilitar `OSLOG_DIAG=true` em agente Windows com `OSLOG_WIN_CHANNELS=System,Application,Security`.
- Rodar coleta quando não há evento novo após o cursor.
- O agente registrava `collect failed collector=oslogs err=oslogs: nenhum evento lido...`; em alguns ambientes essa mensagem podia ser capturada e enviada como log bruto.

Esperado:
- Ausência de evento novo deve ser coleta vazia, sem erro e sem persistir log para IA.

Observado:
- O coletor retornava erro diagnóstico mesmo sem falha real de canal/permissão.

Causa provável:
- `oslogs` adicionava `nenhum evento lido` em `c.errors` com diagnóstico ativo e retornava `formatDiagError` quando não havia eventos.

Hipóteses alternativas:
- Falha real de `wevtutil`, permissão ou arquivo inexistente deve continuar sendo erro diagnóstico.
- Backend pode receber o padrão de agentes antigos até todos atualizarem.

Correção:
- Windows: remover `nenhum evento lido do canal` da lista de erros e retornar `nil` quando não houver evento novo nem falha real.
- Linux/POSIX: retornar `nil` quando não houver evento novo mesmo com diagnóstico ativo; manter erro para arquivo ausente/permissão/falha real.
- Web: filtrar defensivamente `collect failed|collect empty collector=oslogs` em `/v1/logs/raw`.

Critério de fechamento:
- `go test ./internal/platform/collectors/oslogs` passa.
- `php -l api/modules/v1/controllers/LogsController.php` passa no web.
- `./check.sh` passa nos dois repositórios.
- Agente atualizado não gera novo log bruto apenas por ausência de eventos.

## Modelo para novos registros

```txt
## [BUG-001] Título curto

Status:
Severidade:
Área:
Módulo:
Pacote relacionado:
Tela relacionada:
Data:

Objetivo:

Passos para reproduzir:

1.
2.
3.

Resultado esperado:

Resultado observado:

Impacto:

Causa mais provável:

Hipóteses alternativas:

Menor correção possível:

Correção aplicada:

Validação/reteste:

Evidência:

Critério de fechamento:

Validação executada:
```

## Adaptacao deste projeto

- Projeto: `aiceberg_agent`
- Tipo: Agente Go
- Stack detectada: Go agent/CLI, gopsutil, NTP, SNMP
- Regra: adaptar comandos, camadas, testes, fixtures e limites conforme a realidade deste modulo Go.
