# Demandas AIceberg Agent

Backlog do agente desktop/serviço. Este arquivo complementa o backlog do `aiceberg_web` quando a entrega exigir mudanças coordenadas nos dois repositórios.

---

## Organização por pacotes entregáveis

- `PKG-134` Diagnóstico local e controle de falhas de configuração (Agent/Web)

- `PKG-49` Entrega rápida Agentless (implementação/publicação100%; ativação/homologação/SLA separados)
- `PKG-32` Canal operacional bidirecional com agentes
- `PKG-33` Flush resiliente e backpressure para HUB/Relay
- `PKG-73` Agente/SOC — taxonomia canônica de origem, campos SOC e roteamento seguro para PKG-54
- `PKG-74` Agente/Web — descoberta automática de fontes de logs, aplicações e dependências locais
- `PKG-75` Logs/IA — sinais do agente para triagem recorrente e deduplicação segura
- `PKG-82` Agente/Web — perfil local de performance e evidência de host
- `PKG-83` Agente/Web — garantia de cobertura, integridade e segurança da coleta de logs locais
- `PKG-84` Agente/Web — update resiliente e diagnosticável do agente
- `PKG-115` Modelo universal — contrato local de item, série e pipeline
- `PKG-116` Coletores universais — probes locais e de borda
- `PKG-117` Discovery local, autorregistro e protótipos
- `PKG-120` HUB resiliente — assignment, failover e capacidade
- `PKG-121` SDK de extensões seguras e teste pré-ativação

---

## [PKG-134] Agente/Web — diagnóstico local e controle de falhas de configuração

**Status — escopo de implementação 100% concluído em 08/11/2026.** L01–L05
entregues no Agent/Web com guard complementar da API-006 no Runtime. Main de
implementação/commit explicitamente autorizada. Evidência:
`docs/evidence/pkg134-local-20261108.md`. Publicação, homologação e piloto separados.

### Base existente e evidência da lacuna

- [x] ConfigSync tem backoff para transporte/HTTP/leitura e preserva a validação de
  segurança da configuração. Reutilizar esse mecanismo, sem implementar outro.
- [x] config-report legado entrega status/message/config_version/config_hash;
  Web já persiste/apresenta resultado e mantém autenticação/isolamento/auditoria.
- [x] Contenção de notificações repetidas foi entregue separadamente no Web
  (371cc0d0 e correções posteriores de concorrência). Não reconstruir a política
  central nem confundir supressão de envio com controle da tentativa no agente.
- [x] Store de prefs, infraestrutura de transporte e mecanismos de update existem;
  não incluir novos coletores, self-update ou engine de alertas neste pacote.

A falha de aplicação anteriormente retornava sem programar espera. Agora o
ConfigSync/ingest próprio separa consulta, aplicação e relato, reusando retry,
contratos e persistência existentes. Diagnóstico/recuperação possuem estado e
recibos limitados. Incidente289 é histórico; sua causa física não foi comprovada
nem houve reparo do host por esta implementação.

### Implementação entregue

**L01 — Diagnóstico determinístico e contrato aditivo**

- [x] Reproduzir falhas locais com fixtures controladas e definir códigos/causa
  transitória ou persistente, ação segura e evidência sanitizada para pasta válida,
  arquivo conflitante, permissão, disco cheio, filesystem somente leitura e corrida;
  erro desconhecido permanece desconhecido. Especificar fingerprint estável por
  agente/operação/configuração/causa, excluindo timestamps/segredos/dados voláteis.
- [x] Definir metadados versionados e limitados de causa/ação, first_seen/last_seen,
  contagem, tentativa, próximo retry e recuperação, preservando campos legados.
  Mapear produtor, HUB/Relay, API-006 e fallback PHP e qualificar ownership real.
  Definir teto do estado antes da persistência; nenhuma nova migração implícita.

**L02 — Corrigir espera e aplicação segura**

- [x] Conectar o controle de espera existente à falha de persistência/aplicação,
  separando tentativa de aplicar de tentativa de entregar seu relatório. Reutilizar
  jitter/teto para transitórios; limitar reavaliação da mesma causa persistente.
  Configuração/causa nova e recuperação não podem ficar bloqueadas indevidamente;
  preservar última configuração válida, integridade/assinatura e canal de controle.
- [x] Qualificar caminho absoluto/configurado e tratamento seguro de tipo/permissão
  e criação concorrente, reutilizando store/aplicação existentes. Criar somente
  diretório ausente permitido; não apagar/renomear/sobrescrever conflito, seguir
  symlink inesperado, elevar privilégio ou mudar permissões automaticamente.

**L03 — Relatos por mudança/resumo e recuperação**

- [x] Emitir primeira falha, mudança material e recuperação; suprimir relato igual
  a cada ciclo. Resumo no máximo uma vez por 30min/fingerprint, com contagem,
  mantendo política de notificação/destinatários/deduplicação no servidor.
- [x] Manter estado mínimo, limitado e recuperável usando infraestrutura existente,
  com retenção de 30 dias para resolvidos e teto definido em L01, sem depender só
  do diretório que falhou. Se indisponível, usar memória limitada e indicar perda
  de durabilidade. Proteger reboot/crash/timeout de ACK/reentrega sem novo incidente
  artificial; acrescentar métricas de tentativas/supressão/bytes/duração sem alta
  cardinalidade, segredo ou caminho pessoal bruto.

**L04 — Complemento Web e consumidores efetivos**

- [x] Integrar metadados aditivos nas estruturas/superfícies existentes do Web com
  limites, causa/ação, repetição/recuperação e próximo retry; preservar legado,
  tenant e proteção central. Resumo sem mudança não renotifica; recuperação de
  configuração não implica coleta saudável nem encerra outra condição.
- [x] Qualificar preservação por HUB/Relay/API-006/fallback PHP. Adaptar somente
  consumidor efetivo que perca/rejeite dados necessários, antes do rollout do
  produtor; registrar dependência no Runtime se comprovada. Não criar outro motor
  ou declarar mudança de ownership por este planejamento.

**L05 — Integração, testes e consolidação técnica**

- [x] Testar relógio/rede/filesystem controlados: primeira falha, 1000 ciclos iguais
  sem novo relato antes da janela, resumo único, mudança de causa/configuração,
  recuperação, reinício, estado não gravável, reentrega/concorrência/clock skew e
  retorno de versão. Cobrir Windows/Linux e demais SO afetados; agentes antigos/
  novos e Web antigo/novo, HUB/Relay e consumidores. Sem host real como fixture.
- [x] Revisar escopo/SOLID/contratos/rollback/retenção, atualizar manuais/testes/
  runbooks e TELAS se aplicável; rodar validação focada por lote e checks completos
  dos repositórios alterados ao100% de implementação, com commit por pacote.
  Comparações locais reproduzíveis servem de baseline, não de ganho produtivo.

### Publicação — gate separado

Após implementação, validação e autorização, publicar consumidores compatíveis
antes do agente quando necessário; gerar/assinar/publicar os compactados oficiais
antes de solicitar update remoto. Registrar versões/commits/workflows, hashes e
smokes, sem sobrescrever artefato nem atualizar frota por esta reconciliação.

### Homologação — gate separado

Comprovar contrato e fluxo integrado em ambiente controlado autorizado, versões
antigas/novas, transporte direto/HUB/Relay, autenticação/tenant, estados/erros,
repetição/recuperação e rollback. Separar saúde de configuração, agente e coleta.
Registrar versões, SO, esperado/observado e evidências sanitizadas, sem criar
falha destrutiva em host real nem presumir causa física no incidente289.

### Piloto e medição operacional — gate separado

Depois da homologação e publicação autorizadas, piloto limitado com agente real,
observando condição/recuperação segura e proteção central preservada. Comparar
mesmos ciclos/baseline antes/depois: tentativas/requisições/bytes, consultas/envios,
tempo até diagnóstico/recuperação e CPU/RSS/disco. Registrar janela/amostra/versões,
rollback e aceite humano; ganho percentual só após medição. Nenhum piloto ou reparo
de host foi executado nesta tarefa. Conclusão do código não depende de declarar
um ganho operacional ainda não medido.

### Limites e rollback

Sem IA, shell arbitrário, engine de alertas na borda, novo schema SQL ou expansão
para falhas gerais/coletores/self-update. Rollback funcional futuro: versão assinada
anterior e capability/política quando disponível, preservando configuração/outbox/
estado e defesa central. Não limpar arquivos/filas nem descartar histórico.
Implementação local concluída com revisão/checks e commit exclusivo. Reversão de
software deve preservar estado e consumidor compatíveis, sem apagar dados. Sem
push/publicação, homologação, piloto ou atualização de frota nesta entrega.

---

## [PKG-121] SDK de extensões seguras e teste pré-ativação

Status: planejado. Prioridade: P1. Coordenação: Web PKG-121 e Runtime PKG-25.

### Objetivo

Permitir novos coletores locais sem rebuild do núcleo, mantendo manifesto assinado, compatibilidade, permissões mínimas, budgets e isolamento.

### Lotes

- [ ] definir SDK/manifesto versionado, capabilities, SO/arquitetura, assinatura e revogação;
- [ ] executar extensão em processo/sandbox isolado, nunca como shell genérico embutido;
- [ ] limitar CPU, memória, tempo, saída, filesystem e rede por capability;
- [ ] reutilizar o adapter produtivo em modo de teste efêmero sem persistir/ativar configuração;
- [ ] preservar upgrade/rollback do agente sem perder configuração compatível;
- [ ] testar extensão travada, maliciosa, incompatível, revogada e com saída excessiva.

### Aceite e rollback

Falha de extensão não derruba o agente nem bloqueia coletores essenciais; manifesto inválido não executa. Rollback desativa a extensão/versão por configuração assinada ou retorna ao agente anterior.

## [PKG-120] HUB resiliente — assignment, failover e capacidade

Status: implementação parcial de sondagem manual; assignment/HA e aceite físico pendentes. Prioridade: P0. Coordenação: Web PKG-120 e Runtime PKG-24.

### Objetivo

Fazer HUBs participarem de grupos com assignment versionado, lease/fencing, capacidade e redistribuição automática sem duplicar checks; permitir que o HUB delegue a coleta a relays autorizados com alcance comprovado ao item.

### Lotes

- [ ] anunciar identidade, versão, capabilities, capacidade, carga e heartbeat do HUB;
- [ ] aceitar assignment assinado/versionado com epoch, lease, fence e ack idempotente;
- [ ] interromper trabalho ao perder lease/fence e preservar outbox para o novo owner;
- [ ] suportar drain, afinidade, rebalanceamento e retomada sem recriar check;
- [ ] expor atraso, fila, saturação, drops, último sucesso e motivo de rejeição;
- [ ] testar queda, partição, clock skew, assignment concorrente, restart e storm de failover.

### Incremento manual implementado —20/11/2026

Candidata0.8.72-rc.1 assinada e instalada em20/11 no HUB84 (config73) e
relay424 (config5), ambos version_confirmed e sem update pendente.
Comando diagnose_snmp_get só manual, com referência de execução; GET fixo após
falha recente real do HUB, mesma versão/scalar/perfil e executor autorizado.
Controle positivo explicitamente permitido não simula falha. PHP mantém scope
próprio, claim/TTL/primeiro resultado sob lock, sem observação/check/owner novo.
HUB autentica encaminhamento HMAC, API exige identidade assinada do relay.
Fila Bolt exclusiva no relay, sem fallback memória/cloud, job/segredo na fila,
ou ACK sem accepted=true. Reinício/falha retransmitem apenas resultado, não GET.
Testes PHP/Go, UDP local end-to-end e MySQL concorrente permanecem sintéticos.
Piloto físico separado: controle237/238(v2c),239/240,241/242,243/244(v1),
com valor válido no relay→HUB→API. Sensor Catalão245/246 e247/248 timeout com
perfis existentes30/29 em HUB e relay, GET enviado sem resposta. Recortes Web
restaurados/desligados,15ICMP ok/60s, rotina/observações preservadas. Web exigiu
sessão autenticada recente no gate de versão para lidar com inventário atrasado.
Evidência: Web docs/evidence/sada-relay-snmp-pilot-20261120.md. Não fecha
assignment/epoch/lease/fence, eleição automática, HA ou PKG-120 completo.
Procedimento/retencão/rollback: Web docs/operations/relay-snmp-probe-pilot.md.

### Complemento SADA — delegação HUB → relays,19/11/2026

Escopo autorizado de planejamento; encaminhamento relay→HUB existente não comprova delegação implementada. Depende do PKG-116 para executar SNMPv1 efetivamente no coletor escolhido e dos contratos Web PKG-120/Runtime PKG-24 para assignment e autoridade.

- [ ] Distinguir falha da consulta pelo HUB, HUB offline e indisponibilidade do equipamento; timeout não prova sozinho problema de versão, credencial ou queda do sensor.
- [ ] Após falhas confirmadas do check no HUB, selecionar quantidade limitada de relays ativos do mesmo cliente/local/rede autorizada, com capacidade e versão compatíveis. Proximidade por IP/latência não substitui leitura real do sensor; não transmitir probes ou segredos para toda a frota.
- [ ] Delegar probe tipada e limitada por alvo/OID/versão, com segredo por referência/entrega restrita ao executor autorizado, prazo, rate limit e correlação; sem shell genérico remoto.
- [ ] Eleger o relay com leitura válida e manter um responsável por check por período configurado, sob assignment/epoch/lease/fence/ack; impedir coleta autoritativa concorrente do HUB e dos relays, sem refazer a eleição a cada ciclo.
- [ ] Preservar check_id, histórico e recorrência do alarme. Relay reporta ao HUB e este ao AIceberg, com identidade autenticada do executor, horário e versão efetiva; não exigir conexão direta do relay com o AIceberg nem criar check/alarme novo por origem.
- [ ] Renovar, revogar e transferir assignment em falha/drain/retorno com critérios e cooldown explícitos; resultado tardio do owner antigo não recupera autoridade nem sobrescreve estado mais recente. Se o HUB estiver offline, preservar resultados na outbox até a reconexão, sem confirmar entrega inexistente.
- [ ] Testar isolamento de cliente/recurso, segredo/redaction, perda/expiração de lease, dois relays concorrentes, dono antigo, partição, reinício, mensagens duplicadas/reordenadas e reconexão, sem perder observações ou duplicar alarmes.

### Gate bloqueante — delegação com sensor real antes da publicação

- [ ] Cumprir a qualificação SNMPv1 real do PKG-116 antes da promoção geral; candidata assinada de piloto e atualização restrita SADA autorizadas para possibilitar o teste na rede. ICMP/fixture não comprovam resposta SNMP física. Temperatura entregue exige qualificação própria.
- [ ] Antes de publicar/promover a delegação, em piloto restrito autorizado comprovar falha da consulta pela origem HUB e resposta SNMP real pelo relay elegível, qualificando temperatura quando entregue, sem desligar o sensor; registrar topologia, coletor, OID, versão, valor, unidade/escala, horários, correlação e assignment/fence.
- [ ] Demonstrar pelo menos três ciclos consecutivos relay→HUB→AIceberg na cadência configurada, com observações frescas no check original e um único coletor autoritativo/alarme; validar troca/retorno controlados e retomada da outbox sem perda ou duplicação.
- [ ] Manter publicação geral bloqueada até evidências sanitizadas e aceite do piloto. Disponibilização/instalação de candidata de teste segue procedimento vigente de artefatos assinados e autorização de piloto; não equivale a promoção geral.

### Aceite e rollback

HUB e relays nunca executam simultaneamente o mesmo assignment autoritativo válido; queda converge dentro do SLO e preserva observações. Rollback desativa a delegação/rebalanceamento e restaura o assignment anterior assinado com fencing, preservando outbox, checks, histórico e isolamento; não apaga dados.

## [PKG-117] Discovery local, autorregistro e protótipos

Status: evento local e emissão de retirada de interface implementados para Linux e Windows em 24/11/2026; ensaio real Windows e demais fontes pendentes. Prioridade: P1. Coordenação: Web PKG-117 e Runtime PKG-21.

A geração do evento usa a identidade normalizada da prévia de inventário Web,
com fixtures compartilhadas para as seis fontes. Formato de MAC, caminhos e
Unicode foram verificados sem alterar a impressão digital usada nas aprovações.
No Linux, o Agente autentica o envio com a identidade já usada pela ingestão e
enfileira a retirada explícita emitida pelo kernel. Uma interface que ainda
aparece com o mesmo MAC, alteração de nome ou estado e inventário ausente
não geram retirada. O teste isolado confirmou a remoção real de uma interface
virtual e a ausência de evento ao colocá-la administrativamente em down.
O coletor exige habilitação explícita de cliente e ativo no serviço.
No Windows, a retirada exige uma notificação de exclusão e confirmação
na tabela de interfaces do sistema. Remover apenas a família IPv4/IPv6
enquanto a interface permanece presente não inicia retirada. Compilação
Windows e testes da reconciliação passaram. O ensaio de retirada em host
Windows continua pendente.
No Linux, a retirada de discos e partições usa eventos de remoção de
bloco do kernel e a ausência confirmada no inventário sysfs. A identidade
usa o nome publicado em `disk.io_stats`. O teste isolado criou e removeu
um dispositivo de bloco virtual próprio e confirmou o tombstone gerado
pelo kernel, inclusive seu reenvio quando o outbox ficou indisponível.
O ensaio não cobriu a desconexão de disco físico ou de partição.
Na retirada de sensores hwmon, uma falha ao gravar no outbox mantém o
coletor ativo. O reenvio preserva a identidade e o horário, exige confirmação
atual da ausência e descarta eventos vencidos.
Após gravar no Bolt, as retiradas de interfaces, discos e sensores
sobrevivem à reabertura da fila. O ensaio HTTP conferiu os mesmos IDs,
horários e payloads após perda de ACK e troca de credenciais.
No Windows, a retirada de serviço exige que o Service Control Manager
confirme a exclusão de um nome conhecido. Um serviço parado ou ocultado
por falta de permissão permanece no inventário de referência. O Agente
revalida a exclusão antes de reenviar um evento que falhou no outbox.
O ensaio em host Windows com remoção real ainda está pendente. Em Linux,
o Agente usa o inventário de unidades instaladas do systemd e só emite a
retirada de um serviço antes observado quando o systemd confirma ausência
no estado de carga. Unidades apenas paradas, desabilitadas ou mascaradas
permanecem preservadas. O ensaio com systemd real confirmou remoção,
reenvio do evento original após falha do outbox e retorno com a mesma
identidade. A qualificação em produção segue pendente. Para aplicações Debian,
o Agente já emite retirada de pacotes antes instalados após confirmar a
desinstalação no dpkg.
Uma remoção completa exige ausência em duas leituras completas consecutivas.
Um ensaio isolado com dpkg real confirmou remoção, reinstalação e purge.
Para RPM, duas leituras completas sem o pacote confirmam a retirada da
identidade por nome e arquitetura. Versões instaladas em paralelo mantêm
a identidade presente até a última ser removida. O ensaio usou uma cópia
descartável da base RPM real. Para aplicações Windows, o Agente acompanha
as chaves de desinstalação da máquina nas visões 32 e 64 bits do Registro.
Duas capturas completas sem a chave confirmam a retirada de um nome
observado sem ambiguidade. Mudança de chave com o mesmo nome, renomeação
em uma chave presente e leitura incompleta preservam o item. Falta o
ensaio de desinstalação e reinstalação em host Windows.
O Agente também produz retirada opt-in para arquivos identificados pelo
Discovery de logs. Só acompanha candidatos que chegaram à fila e exige
duas confirmações da ausência separadas por pelo menos dois minutos,
com o mesmo diretório acessível. Retorno após rotação dentro do intervalo,
troca de diretório e falha de leitura preservam a fonte. No Windows, a
descoberta confirma o registro dos canais padrão e configurados no Event
Log. A retirada exige duas respostas nativas de canal inexistente, com o
mesmo intervalo. Canais desabilitados continuam registrados; falhas de
acesso interrompem a confirmação. Falta o ensaio em host Windows.
Em Linux, fontes vinculadas a uma unit do systemd e ao socket do
journald exigem a ausência do caminho já observado e a confirmação
`not-found/inactive` no gerenciador local. A consulta também foi
conferida em um host Linux, com uma unit ativa e outra inexistente.
O ensaio Linux em VM existente criou uma unit temporária, confirmou
a retirada quando o gerenciador deixou de reconhecê-la e redescobriu a
mesma identidade após sua restauração. A unidade de teste foi removida ao final.
Sinais de processo, listener e outros runtimes seguem sem retirada
automática até existir confirmação local de exclusão da fonte de log.

### Objetivo

Descobrir interfaces, discos, sensores, serviços, aplicações e fontes locais com identidade estável, limites e evidência suficiente para o Runtime materializar protótipos.

### Lotes

- [x] definir evento de discovery com fingerprint, fonte, confiança, freshness e tombstone;
- [ ] implementar inventário incremental e bounded por coletor, sem comando arbitrário;
- [ ] autorregistrar agente/HUB com identidade forte, quarentena e rotação;
- [ ] detectar rename, desaparecimento temporário e retorno preservando identidade;
- [ ] aplicar backpressure, jitter, cardinalidade e redaction antes do transporte;
- [ ] cobrir Windows/Linux, privilégio insuficiente, volume alto e fonte instável.

### Aceite e rollback

Redescoberta não duplica; desaparecimento temporário não apaga histórico; evento sem identidade/confiança suficiente fica em quarentena. Rollback desativa o coletor por configuração.

## [PKG-116] Coletores universais — probes locais e de borda

O despacho normal do Agente usa o registro de adaptadores, valida o
orçamento de cada coleta e inclui recibos e amostras canônicas. Os
protocolos adicionais exigem configuração compatível. Distribuição das
credenciais de rotina pelo HUB e ativação em campo seguem em lotes separados.

O receptor de traps do HUB usa habilitação explícita e fila dedicada. O Bolt
controla capacidade e replay no mesmo commit. A instalação requer configuração
local e ingestão compatível no servidor.

A coleta SNMP preserva as categorias de timeout, autenticação e falha de
transporte sem enviar erros brutos da biblioteca nas observações. A fixture
verifica as saídas parciais e finais; o piloto físico segue pendente.

Contratos locais de políticas e recibos, seletores e projeção de latência e
métricas dos novos protocolos foram isolados no núcleo. A integração dos
adaptadores, da fila e das credenciais fica em entrega separada.

Status: implementação parcial; candidata rc.1 instalada no HUB84, teste físico pendente. Prioridade: P1. Coordenação: Web PKG-116 e Runtime PKG-20.

### Objetivo

Executar somente os protocolos que precisam de proximidade do host/rede, com contrato uniforme e privilégio mínimo.

### Cobertura

- pull/passive check autenticado do agente quando a topologia permitir;
- SNMPv1/v2c/v3 por perfil explícito: GET e WALK compatíveis com a versão; GETBULK somente em v2c/v3; receptor de traps no HUB;
- JMX direto e Jolokia; Prometheus/OpenMetrics;
- queries read-only de banco com segredo referenciado;
- IPMI; SSH por comandos tipados; Telnet legado opt-in;
- runner sintético/browser isolado quando implantado no ponto de presença.

### Lotes

- [ ] adotar contrato comum de probe/amostra/evento/erro do Runtime;
- [ ] separar adapters por pacote interno e capability, sem `switch` monolítico;
- [ ] implementar pools, timeout, rate limit, cancelamento, backpressure e circuit breaker;
- [ ] impedir segredo, linha sensível ou payload completo em log/outbox;
- [ ] entregar fixtures/simuladores e matriz real por protocolo/SO/fabricante;
- [ ] medir footprint e pausar adapter que exceda budget sem afetar o agente.

### Incremento local —19/11/2026

- [x] Seleção nativa explícita v1/v2c/v3, rejeição de versão desconhecida, GET v1 isolado e WALK por GETNEXT sem GETBULK; teto256requisições/cadência25ms, deadline, cancelamento, limite de linhas e proteção de loops.
- [x] OIDs/PDU/tipos indisponíveis não fabricam zero; versão efetiva no runtime_applied e regressões v2c/v3.
- [x] Probe ENTITY-SENSOR-MIB opt-in: somente Celsius com metadados, estado e frescor do dispositivo qualificados; lacunas são unknown/null. Precisão negativa fica explicitamente não suportada.
- [x] Candidata0.8.71-rc.1 e comparação de versões corrigida para promoção posterior, sem forçar downgrade; DTO/probe/visibilidade restritos no Web.
- [x] Construir/verificar/publicar candidata assinada0.8.71-rc.1 e confirmar instalação/version_confirmed no HUB84/SADA; demais agentes preservados.
- [x] Diagnóstico real166/167/168 chegou por canal autenticado e terminou skipped_busy, sem consulta SNMP ou temperatura. Janela temporária30s de poll/flush não resolveu; cadência original1/2s restaurada e piloto desligado.
- [x] Correção para0.8.71-rc.2: um comando manual pode aguardar a rotina, com deadline incluindo espera, prioridade sobre novo polling, sem coleta paralela e sem limpar lock alheio ao cancelar. Outro comando é recusado como ocupado; rotina retoma após término.
- [x] Construir/verificar/publicar rc.2 assinada e confirmar instalação/heartbeat/canal/version_confirmed somente no HUB84; atualização71 entregue, sem pendência.
- [x] Executar comparação real no sensor Catalão:170comprovou v1 e171comprovou v2c, ambas snmp_time_budget_exceeded5000ms, sem identidade/temperatura. Troca de versão não resolveu esse alvo; não extrapolar aos demais14.
- [x] Encerrar piloto: parâmetro desligado, configuração privada restaurada, poll/flush1/2s originais, zero observações operacionais e check inativo/counters/agenda intactos. Recibo Web: docs/evidence/snmp-v1-sada-pilot-20261119.md.
- [ ] Qualificar alcance/UDP161/ACL/credencial/origem na rede, demais sensores, três ciclos válidos e referência física. Delegação120 ainda aberta; catálogo universal e promoção geral permanecem pendentes.

Testes com race e UDP real em loopback são sintéticos. Contrato, limites e rollback:
Web `docs/operations/snmp-v1-sensor-pilot.md`. Sem novo schema SQL ou transporte
canônico ativo; profile/config de rotina preservados pelo probe temporário.

### Incremento de diagnóstico básico —20/11/2026

Implementação com testes focados locais aprovados na candidata0.8.71-rc.3: um GET de uptime/identidade,
sem WALK, timeout1500ms/retries0/teto5s; versão real, counters UDP e resposta
SNMP decodificada distintos. Sem erro bruto/segredo no resultado. Fixtures UDP
v1/v2c são sintéticas, sem equivaler ao hardware SADA. O objetivo atualizado do
titular é online/ativo; temperatura opcional. Não há novo aceite físico neste
registro. O Web fixa preset/scalar/versão/referência de perfil no comando.
PKG-116 permanece parcial e delegação relay depende da entrega PKG-120.

### Complemento SADA — SNMPv1,19/11/2026

Escopo autorizado de planejamento; não declara suporte implementado nem leitura real comprovada. Coordenação com Web PKG-116/Runtime PKG-20 e com a delegação do Agent/Web PKG-120.

- [ ] Qualificar sensor real por consulta SNMPv1 no HUB ou agente escolhido para piloto, comparando v2c na mesma origem/endpoint/porta/comunidade. Sem acesso externo, implementar e publicar candidata assinada restrita, atualizar os agentes SADA escolhidos e só então executar o ensaio; perfil v1 só será enviado a candidato compatível. Promoção geral depende de leitura física e aceite.
- [ ] Implementar seleção explícita de v1 no cadastro/contrato Web e no coletor Agent, preservando v2c/v3; rejeitar versão incompatível/sem suporte, sem convertê-la silenciosamente em v2c.
- [ ] Usar GET e GETNEXT/WALK em v1, sem GETBULK; classificar OID/tipo não suportado como indisponível, sem fabricar zero, sucesso ou leitura de temperatura.
- [ ] Preservar limites, cancelamento, segmentação/outbox e identidade do check; registrar versão efetiva, coletor executor, HUB, OID, unidade/escala e horário real, sem comunidade ou credenciais nos logs/recibos.
- [ ] Cobrir v1/v2c/v3, GET/WALK, OID ausente, comunidade/versão incorretas, timeout, dado inválido/vencido e regressões dos coletores atuais.

### Gate bloqueante — sensor real antes da publicação

- [ ] Antes da promoção geral de SNMPv1, obter resposta SNMP real e três ciclos de disponibilidade pela origem autorizada. Temperatura é opcional conforme objetivo atualizado do titular; ao entregá-la, identidade/sysUpTime isolados não bastam. Registrar fabricante/modelo, ativo/check, origem, versão, OID, valor bruto, unidade/escala e valor convertido; confrontar com a leitura local/referência conhecida do equipamento, sem telemetria inventada.
- [ ] Comprovar estabilidade em pelo menos três ciclos consecutivos na cadência configurada. Guardar evidências sanitizadas em local privado e resultados v1/v2c separados; teste pelo servidor cloud sem a mesma rota não substitui esse gate.
- [ ] Após implementar, repetir a leitura com a candidata do agente em piloto restrito autorizado, seguindo assinatura/artefatos e procedimento de piloto vigentes, antes da promoção/publicação geral; confirmar observação persistida, cliente/ativo/check corretos, frescor e preservação do histórico.
- [ ] Bloquear publicação/promoção do suporte novo enquanto faltarem leitura real, testes de regressão, rollback e aceite do piloto; check verde/simulador não substitui sensor físico. O teste externo inicial não exige atualização do agente.

### Aceite e rollback

Cada adapter falha isoladamente, respeita budget e reporta estado explícito. Rollback desabilita capability/adapter e mantém métricas essenciais e transporte.

## [PKG-115] Modelo universal — contrato local de item, série e pipeline

Status: implementação parcial de contratos puros; consumo ativo pendente. Prioridade: P0. Coordenação: Web PKG-115 e Runtime PKG-19.

### Objetivo

Adaptar Collector/Forwarder existentes ao modelo canônico sem quebrar snapshots, permitindo fan-out e preprocessing no Runtime sem duplicar coleta.

### Lotes

- [ ] mapear coletores e payloads atuais para item, série, evento, fonte e schema versionados;
- [ ] emitir identidade estável, unidade, tipo, timestamp, freshness, qualidade e lacuna;
- [ ] separar dado bruto controlado de métricas derivadas e evitar fan-out local duplicado;
- [ ] preservar contratos legados por adapter durante rollout e comparar payload canônico;
- [ ] aplicar budgets/cardinalidade, batching, compressão, retry e backpressure;
- [ ] cobrir compatibilidade com Web/Runtime antigo e rollback de versão.

### Incremento inicial —19/11/2026

Contrato numérico SNMP v1 e adapter de observação existentes implementados em
`internal/domain/monitoring` e `internal/platform/agentless/monitoring_projection.go`.
18vetores compartilhados e PHP real com race aprovados; preservação do payload
legado, escopo e ausência de nova coleta comprovados. Não encerra os lotes acima:
modelo/evento universal, demais coletores, transporte e consumo ativo pendentes.
Contrato coordenado: Web `docs/contracts/monitoring-samples-v1.md`.

Incremento20/11/2026: definição de itens com identidade por cliente/ativo/check,
fontes/tipos explícitos, retenção/frescor limitados e grafo de dependências.
Validação exige autoridade externa, rejeita ciclo, mestre ausente/desabilitado,
profundidade/tamanho excessivos e campos extras/segredos.52vetores idênticos ao
Web, PHP real, race e codec JSON limitado testados; integração ativa não feita.
Contrato: Web docs/contracts/monitoring-definitions-v1.md. Patches Web/Runtime
foram reintegrados após encerrar a intervenção concorrente de logs. Paridade
PHP/Go e regressões de agendadores/análise/controle PHP repetidas com race.
Composição/herança/macros e preview novos ficam exclusivamente no Runtime;
o Agent mantém apenas descrição/validação e não recebe outro motor de templates.
Adapter DescribeSNMPItems descreve a projeção SNMP existente com política e
autoridade explícitas, sem coletar/copiar endpoint/credencial; diagnóstico não
é promovido a definição de rotina. Chaves compatíveis e job intacto testados.
Templates/macros/pipeline, amostras/eventos universais, emissão/transporte,
compatibilidade instalada e fechamento do pacote continuam pendentes.

### Aceite e rollback

Uma coleta possui identidade reproduzível e pode gerar séries dependentes no Runtime; payload legado continua aceito durante a janela. Rollback publica a versão anterior e mantém o adapter de compatibilidade servidor-side.

## [PKG-82] Agente/Web — perfil local de performance e evidência de host

**Status** — implementado no agente em 24/06/2026 como payload aditivo `performance_profile` dentro de `sysmetrics`, coordenado com o `aiceberg_web`. A versão `0.8.22` adiciona `agent_runtime` para separar o consumo do próprio agente do consumo do host.

**Escopo no agente** — emitir evidência local de CPU, memória, disco, rede, top processos, checks de conectividade e lacunas de coleta sem endpoint novo e sem profiler invasivo.

**Contrato** — `performance_profile` contém `schema_version`, `window_sec`, `source`, `resources`, `agent_runtime`, `processes`, `checks` e `gaps`. O payload é opcional e compatível com agentes/web legados.

### Lotes

1) **Payload aditivo**
   - [x] emitir `performance_profile` no snapshot de métricas;
   - [x] adicionar `performance_profile` à allowlist de `metricsKeys`;
   - [x] preservar endpoint `/v1/ingest/metrics` e contrato legado.

2) **Evidência local**
   - [x] derivar recursos de CPU, memória, disco e rede a partir de `sysmetrics`;
   - [x] derivar top processos a partir de `processes`;
   - [x] classificar papel provável do processo (`agent`, `database`, `web`, `application`, `process`);
   - [x] incluir checks de DNS/TCP e flush do agente quando disponíveis.

3) **Segurança e lacunas**
   - [x] sanitizar command line do perfil para token, senha, secret, authorization e bearer;
   - [x] reportar lacunas quando CPU, memória, disco, rede ou processos estiverem desativados/indisponíveis;
   - [x] limitar quantidade de processos, checks e tamanho de texto.

4) **Runtime e footprint do agente**
   - [x] reportar PID, versão, modo, usuário sanitizado, executável, diretório de trabalho e uptime do próprio agente;
   - [x] reportar CPU, memória RSS/percentual e IO do processo do agente;
   - [x] reportar footprint local de outbox, agentless outbox, prefs, config, logs, cache e temporários com status de risco;
   - [x] manter path sanitizado e varredura bounded para não gerar IO pesado.

5) **Validação**
   - [x] cobrir contrato e redaction em `go test ./internal/platform/collectors/sysmetrics`;
   - [x] rodar `./check.sh` no fechamento;
   - [x] validar publicação do pacote coordenada com o web.

### Rollback

- Remover `performance_profile` de `metricsKeys` e publicar versão anterior do agente.
- O web deve continuar aceitando payload sem `performance_profile`.

## [PKG-83] Agente/Web — garantia de cobertura, integridade e segurança da coleta de logs locais

**Status** — implementado no agente em 24/06/2026 como payload aditivo `log_source_health` dentro de `/v1/logs/raw`, gerado originalmente como versão `0.8.22`. A versão `0.8.26` reforça a promoção determinística de sinais de segurança/erro operacional e cobre fixtures de fechamento do PKG-83.

**Atualização 24/06/2026** — validação automatizada POSIX ampliada para tail, truncate/rotação, permissão negada, arquivo inexistente e cursor desalinhado; validação de canal Windows inexistente permanece coberta pela normalização de `channel_missing`.

**Escopo no agente** — reportar health por fonte local de log sem enviar conteúdo bruto: arquivo/canal, cursor, tamanho, mtime, identidade, contadores de linhas lidas, eventos aceitos, eventos descartados, erro, permissão, estado canônico e lacunas.

**Contrato** — cada item de `log_source_health` contém `schema_version`, `source_fingerprint`, `path` ou `channel`, `kind`, `product`, `approved`, `enabled`, `last_scan_at`, `last_read_at`, `last_event_at`, `last_sent_at`, `cursor`, `file_size`, `file_mtime`, `file_identity`, `read_lines`, `accepted_events`, `dropped_events`, `drop_reason`, `last_error`, `permission_status`, `status`, `confidence` e `gaps`.

### Lotes

1) **Health por fonte**
   - [x] reportar health para `OSLOG_FILES`;
   - [x] reportar health para journald quando habilitado;
   - [x] reportar health para `OSLOG_WIN_CHANNELS`;
   - [x] diferenciar `delivering`, `no_new_events`, `permission_denied`, `file_missing`, `channel_missing`, `dropped_by_severity` e `unknown`.

2) **Integridade e segurança**
   - [x] manter prova por fingerprint/contadores sem linha bruta;
   - [x] registrar cursor, mtime, tamanho e identidade de arquivo quando disponível;
   - [x] promover sinais de segurança sem severidade explícita para passar pelo filtro mínimo `error`;
   - [x] manter redaction existente do conteúdo enviado.

3) **Validação**
   - [x] cobrir POSIX com `go test ./internal/platform/collectors/oslogs`;
   - [x] validar build cruzado Windows com `GOOS=windows GOARCH=amd64 go test -c`;
   - [x] cobrir fixtures de SSH, mail/SASL, sudo/PAM, nginx/apache, Windows Security 4625, Sysmon e canal Windows inexistente;
   - [x] cobrir tail de arquivo, truncate/rotação, permissão negada, arquivo inexistente e cursor desalinhado;
   - [x] rodar `./check.sh` no fechamento;
   - [x] publicar pacote `0.8.26` coordenado com o web.

### Rollback

- Publicar versão anterior do agente ou ignorar `log_source_health` no backend.
- A coleta de logs atual continua compatível, pois o payload é aditivo.

## [PKG-84] Agente/Web — update resiliente e diagnosticável do agente

**Status** — implementado parcialmente no agente em 24/06/2026 como complemento do self-update existente. A versão `0.8.25` adiciona cooldown persistido por versão/erro, fingerprint de falha, fingerprint idempotente do report, metadados de tentativa, preflight local e status `rolled_back` quando a versão alvo não confirma após restart no `update-report`. A versão `0.8.27` reforça o preflight com espaço livre de staging, escrita no diretório de instalação, gerenciador de serviço e sinalização de lock/binário por SO. A versão `0.8.28` preserva `.part`, retoma download com `Range` quando possível, valida SHA256 após retomada e registra evidência de retomada. A versão `0.8.29` expõe `rollback_available`/`rollback_version` no estado pendente e nos reports de reconexão/rollback e limpa staging antigo com retenção segura. A versão `0.8.30` falha cedo no preflight quando o diretório de instalação não é gravável e passa a reportar modo do binário, modo do diretório de instalação e checagem explícita de escrita antes de download/apply. A versão `0.8.31` trata `416 Requested Range Not Satisfiable` como parcial inválido, remove o `.part` e tenta download limpo uma vez. A versão `0.8.32` reforça o apply Windows para validar health local opcional após restart e restaurar o backup quando o serviço sobe sem ficar saudável. A versão `0.8.33` limita comandos Linux caros de inventário/repositório e limpa estado pendente local obsoleto da versão atual antes de processar novo update. A versão `0.8.34` remove `dnf/yum` do hot path de métricas por padrão, lê repositórios por arquivos `.repo` e deixa `dnf updateinfo` apenas por opt-in explícito. A versão `0.8.35` limita a varredura de processos e limpa pendência local da versão atual durante o snapshot operacional. A versão `0.8.36` remove inventário completo `rpm/dpkg` do hot path por padrão, mantendo opt-in explícito. Em produção, agentes InspectApp `1`, `4`, `18`, `19`, `70` e `71` confirmaram `0.8.31`, `version_confirmed`, sem payload pendente e sem loop quente de update. O agente Linux `1` expôs falha real de `.part` inválido/`416`, corrigida na `0.8.31`; o Windows `71` validou probe controlado de SHA inválido sem substituir binário; o agente Linux `4` expôs resíduo local de `.pending_update.json` e coleta Linux demorada, endereçados nas versões `0.8.33`, `0.8.34`, `0.8.35` e `0.8.36`. Fechamento completo ainda depende de cenários reais destrutivos/controlados de permissão, lock, EDR/AV, restart e rollback.

**Atualização 2026-07-03** — a versão `0.8.40` corrige instalações Linux em caminhos alternativos, fazendo o update aplicar no mesmo binário executado pelo serviço (`AICEBERG_UPDATE_BIN_DST` com fallback para `AICEBERG_AGENT_BIN` e `/usr/local/bin/aiceberg_agent`).

**Escopo no agente** — impedir loop quente de update após restart, preservar diagnóstico de falha e enviar evidência objetiva para o web sem novo endpoint.

**Contrato** — `update-report` pode conter `report_fingerprint`, `update.report_fingerprint`, `update.attempt_count`, `update.cooldown_until`, `update.cooldown_until_unix`, `update.last_error_fingerprint`, `update.last_reason_code`, `update.failure_class`, `update.download_file`, `update.download_sha256`, `update.rollback_available` e `update.rollback_version`.

### Lotes

1) **Cooldown persistido**
   - [x] persistir `.update_cooldown.json` no diretório de update;
   - [x] bloquear nova tentativa da mesma versão durante cooldown mesmo após restart;
   - [x] registrar tentativa, motivo, fingerprint sanitizado e prazo de próximo retry;
   - [x] limpar cooldown após download sem comando de apply ou apply despachado com sucesso.

2) **Diagnóstico no report**
   - [x] anexar `attempt_count`, `cooldown_until`, `last_error_fingerprint` e `last_reason_code` em falhas/cooldown;
   - [x] anexar `report_fingerprint` estável por versão, status, SHA, estágio e fingerprint de erro;
   - [x] preservar `failure_class`, arquivo, SHA e estágio já existentes;
   - [x] reportar `rolled_back` quando a reconexão mantém a versão anterior após tentativa de apply;
   - [x] reportar `rollback_available` e `rollback_version` no estado pendente e nos reports de reconexão/rollback;
   - [x] cobrir contrato em `go test ./internal/domain/usecase -run 'SelfUpdate|Update'`.

3) **Preflight local**
   - [x] validar executável atual, diretório de instalação, staging gravável, espaço livre, gerenciador de serviço e shell/PowerShell antes do download/apply;
   - [x] reportar `preflight_failed` com `failure_code`, recomendação, checks e lacunas por SO;
   - [x] validar em teste controlado `staging_not_writable`, `staging_low_space` e `install_dir_not_writable` sem tocar em host produtivo;
   - [x] manter lacunas explícitas para restart real, lock de binário, EDR/antivírus e SELinux/AppArmor quando só podem ser provadas no apply real.

4) **Download resiliente**
   - [x] preservar arquivo parcial `.part` quando o download falha antes de finalizar;
   - [x] retomar download com header `Range` a partir do tamanho parcial quando o servidor responder `206 Partial Content`;
   - [x] reiniciar download limpo quando o servidor responder `200 OK` sem suportar retomada;
   - [x] validar SHA256 do arquivo completo antes de finalizar e expor `download_resumed`/`download_resume_offset` no report.

5) **Staging e retenção**
   - [x] preservar metadados de rollback do update pendente sem expor segredo;
   - [x] limpar diretórios antigos de staging após validação do artefato, preservando versão atual, diretórios recentes e arquivos de estado `.pending_update.json`/`.update_cooldown.json`;
   - [x] validar falha controlada de apply com `launcher_exit_code`, cooldown persistido e pendência limpa;
   - [x] cobrir limpeza por teste unitário sem apagar estado operacional.

6) **Fechamento**
   - [x] rodar `./check.sh`;
   - [x] publicar pacote coordenado com o web;
   - [x] validar piloto real Windows/Linux de update bem-sucedido no cliente InspectApp em 24/06/2026: agentes `1`, `4`, `18`, `19`, `70` Linux AMD64 e agente `71` Windows AMD64 confirmaram `0.8.24`;
   - [x] validar piloto real complementar `0.8.25` no cliente InspectApp em 24/06/2026: agentes `1`, `4`, `18`, `19`, `70` Linux AMD64 e agente `71` Windows AMD64 terminaram em `version_confirmed`, sem `send_configuration` pendente e sem `pending_update_payload`; o agente `4` confirmou após reenvio controlado do manifest;
   - [x] validar piloto real complementar `0.8.29` no cliente InspectApp em 24/06/2026: agentes `1`, `4`, `18`, `19`, `70` Linux AMD64 e agente `71` Windows AMD64 permaneceram em `version_confirmed`, sem `send_configuration` pendente, sem `pending_update_payload` e sem loop quente de `update-report`;
   - [x] gerar pacote `0.8.30` com reforço de preflight de diretório de instalação não gravável e metadados de permissão do binário/diretório;
   - [x] gerar pacote `0.8.31` com limpeza de `.part` inválido quando retomada por `Range` recebe `416`;
   - [x] gerar pacote `0.8.32` com healthcheck opcional no apply Windows antes de confirmar restart saudável e rollback para backup se o health local não responder;
   - [x] gerar pacote `0.8.33` com timeout curto para inventário/repositórios Linux e limpeza de pendência local obsoleta da versão atual;
   - [x] gerar pacote `0.8.34` removendo `dnf/yum` do hot path de métricas e mantendo updateinfo apenas por opt-in explícito;
   - [x] gerar pacote `0.8.35` com limite curto para snapshot de processos e limpeza de pendência local da versão atual durante snapshot operacional;
   - [x] gerar pacote `0.8.36` removendo inventário completo `rpm/dpkg` do hot path de métricas por padrão;
   - [x] gerar pacote `0.8.40` com destino de update Linux alinhado ao binário em execução;
   - [x] validar piloto real complementar `0.8.31` no cliente InspectApp em 24/06/2026: agentes `1`, `4`, `18`, `19`, `70` Linux AMD64 e agente `71` Windows AMD64 terminaram em `version_confirmed`, sem `send_configuration` pendente e sem `pending_update_payload`;
   - [x] validar falhas reais/controladas de update: Linux `1` com `.part` inválido seguido de `416` corrigido por download limpo na `0.8.31`; Windows `71` com SHA inválido em probe controlado, registrando falha terminal e limpando pendência sem apply;
   - [x] validar piloto real Windows/Linux com falhas induzidas/conhecidas de permissão, lock, EDR/AV, restart e rollback. Fechado em 24/06/2026 com combinação de falhas reais/controladas e simulações destrutivas seguras: Linux `1` expôs `.part` inválido seguido de `416` e recuperou com download limpo; Windows `71` validou SHA inválido sem apply e limpou pendência; testes focados cobriram staging sem escrita, espaço insuficiente, diretório de instalação sem permissão, hash/assinatura inválidos, timeout de download, falha de apply, cooldown, rollback por mismatch/reconexão e preservação de backup/estado. O rollout `0.8.36` confirmou agentes InspectApp `1`, `4`, `18`, `19`, `70` e `71` sem payload pendente.

### Rollback

- Apagar `.update_cooldown.json` somente após diagnóstico aprovado ou publicar versão anterior do agente.
- O web pode ignorar os campos aditivos de update-report sem quebrar o fluxo atual.

## [PKG-32] Canal operacional bidirecional com agentes

**Problema a resolver** — o agente depende demais de polling HTTP para comandos, atualização, coleta sob demanda e confirmação de estado. Isso aumenta latência, gera filas grandes e dificulta diagnóstico de falhas como update, sudoers, restart e reconexão.

**Escopo no `aiceberg_agent`** — implementar o lado do agente para manter um canal operacional persistente ou quase persistente com o servidor, preservando o fluxo atual como fallback.

**Dependências** — `aiceberg_web` PKG-32, PKG-19 (self-update, self-healing, modo remoto), PKG-20 (envio incremental), configuração segura do serviço por SO.

### Lotes propostos

1) **Contrato local do canal**
   - [x] definir envelope canônico para mensagens de sessão, comando, ACK, progresso, resultado, erro e heartbeat;
   - [x] incluir `agent_id`, hostname, versão, modo, capacidades, `command_id`, `correlation_id`, timestamps e tentativas;
   - [x] garantir topologia por modo: `direct -> AIceberg`, `hub -> AIceberg`, `relay -> hub -> AIceberg`;
   - [x] preservar os contratos HTTP atuais (`ping`, `bootstrap`, `config`, `selfheal`, `update-report`, `ingest`) como compatibilidade e fallback;
   - [x] separar domínio/contrato de transporte para permitir WebSocket no MVP e polling como fallback.

2) **Cliente persistente outbound**
   - [x] criar cliente outbound para o endpoint operacional do modo `direct` (`/v1/agent/channel`, HTTP equivalente ao canal persistente);
   - [x] em modo `relay`, conectar somente ao `hub` configurado, sem exigir conexão direta com AIceberg;
   - [x] em modo `hub`, aceitar conexões dos relays e manter conexão do hub com AIceberg;
   - [x] autenticar com token atual do agente e validar rejeições de credencial;
   - [x] implementar reconnect com backoff, jitter e limite de logs;
   - [x] manter heartbeat e medição de latência.

3) **Fallback para polling atual**
   - [x] manter `/v1/agent/ping`, `/v1/agent/config` e self-healing atuais funcionando;
   - [x] ativar fallback automático quando o canal persistente cair;
   - [x] manter compatibilidade com relays que ainda operem apenas pelo fluxo atual durante rollout;
   - [x] evitar execução duplicada do mesmo comando entre canal novo e polling por idempotência local.

4) **Executor de comandos com progresso**
   - [x] padronizar ACK imediato ao receber comando;
   - [x] enviar eventos de progresso para comandos longos;
   - [x] retornar resultado final com evidência sanitizada;
   - [x] aplicar timeout, cancelamento cooperativo e retry controlado.

5) **Coleta sob demanda e streaming**
   - [x] permitir coletas sob demanda sem bloquear o loop principal;
   - [x] enviar payloads grandes em chunks com checksum/ordem;
   - [x] reportar progresso de inventário, health, network capture e agentless;
   - [x] respeitar quotas de CPU, memória, disco e rede.

6) **Update remoto com handshake**
   - [x] expor etapas de update: precheck, download, validação, apply, restart agendado, reconexão e confirmação;
   - [x] reportar falhas específicas: permissão/sudoers, pacote inválido, binário ausente, restart falho, versão não confirmada;
   - [x] confirmar versão nova após restart sem depender apenas de bootstrap manual;
   - [x] manter rollback local e fallback para o launcher atual.

7) **Segurança local**
   - [x] manter allowlist de comandos suportados;
   - [x] bloquear shell arbitrário;
   - [x] sanitizar logs, paths, tokens e payloads;
   - [x] limitar comandos destrutivos a fluxos com aprovação explícita na web.

8) **Operação e suporte**
   - [x] criar diagnóstico local do canal (`doctor` ou comando remoto equivalente);
   - [x] expor status do canal no health local;
   - [x] documentar instalação, variáveis e troubleshooting;
   - [x] adicionar testes unitários e integração local simulando servidor.

### Critérios de aceite

- [x] agente conecta outbound ao servidor e mantém presença/heartbeat;
- [x] comando simples executa via canal persistente com ACK e resultado;
- [x] queda do canal volta para polling sem perder comando pendente;
- [x] update remoto reporta etapa detalhada e confirma versão após reconexão;
- [x] payload grande é transmitido em chunks sem travar coleta principal;
- [x] `./check.sh` passa antes de qualquer release do agente.

### Fora de escopo inicial

- abrir porta inbound obrigatória no cliente;
- SSH reverso genérico;
- shell remoto arbitrário;
- conexão direta obrigatória de `relay` com AIceberg;
- remoção dos contratos HTTP atuais antes de homologação;
- remover polling atual antes de homologação e rollback.

---

## [PKG-33] Flush resiliente e backpressure para HUB/Relay

**Problema a resolver** — em topologia `relay -> hub -> AIceberg`, o Hub pode acumular backlog quando a API demora para responder a `/v1/ingest/metrics`. No incidente observado em 2026-04-29, o Relay `RHL-CTG-HML-01` coletava métricas e fazia flush para o Hub, mas o Hub registrava `context deadline exceeded`, `queue_items > 1000` e `flush_err` crescente. Como o flush atual trabalha com lote FIFO e só confirma ACK depois de enviar todos os grupos, falhas temporárias de um endpoint podem atrasar métricas recentes e degradar a visibilidade do relay.

**Escopo no `aiceberg_agent`** — tornar o flush do Hub resiliente sob carga, com timeout/batch configuráveis, ACK granular, retry controlado, priorização de telemetria operacional e diagnóstico local suficiente para operação sem desligar coletas essenciais.

**Dependências** — `aiceberg_web` PKG-33, PKG-20 (envio incremental), PKG-32 (canal operacional), publicação oficial de artefatos conforme `GOVERNANCA.md`.

### Correção do incidente03/11/2026 — retomada após progresso

- [x] Separar contagem de grupos não despachados das recusas/transporte.
- [x] Retomar em1s somente HUB com ACK positivo e lote cheio/deferral.
- [x] Preservar limites8s/8threads, ACK, auth/identidade/rota e cooldown.
- [x] Impedir que os ciclos adicionais antecipem pruning.
- [x] Cobrir rollback por flag, direct/relay e retomada com Bolt/HTTP/race.
- [x] Consolidar check/review e commit exclusivo desta correção.
- [x] Publicação0.8.61 e piloto autorizado somente HUB84 em04/11; métricas0–1s e Agentless ativos.

Não fecha o PKG-33 nem altera o planejamentoPKG-134. Não gera artefato/update
nesta etapa. Evidência: docs/evidence/hub-outbox-catchup-20261103.md.

### Envio por chegada em worker dedicado — 04/11/2026

Solicitado pelo usuário após a correção local610fead: ao receber dados dos
Relays, persistir e sinalizar envio sem aguardar o timer nem bloquear coletas.

- [x] Notificar somente após Append bem-sucedido; sinal não bloqueante/coalescido.
- [x] Um worker proprietário dos dois flushes no HUB; timer inline desativado.
- [x] Preservar fila/ACK, auth/identidade/rota, cooldown, limites8s/8threads/8MiB.
- [x] Manter fallback periódico, retomada de backlog e frequência normal de pruning.
- [x] Serializar configuração própria no loop principal; relay config segue PendingConfigStore.
- [x] Cancelar/juntar worker e encerrar listener; preservar pendentes em disco.
- [x] Testar Relay→HUB→Bolt→API bloqueada, rajada64relays, cancelamento e race.
- [x] Consolidar check/review e commit exclusivo.
- [x] Publicar0.8.61 e validar piloto autorizado HUB84; relay124/Agentless, SHA do binário e API sem500 no recorte.

Rollback porHUB_OUTBOX_ASYNC_ENABLED=false mantém o fluxo local610fead.
Não fecha o PKG-33/134 nem autoriza push ou update remoto.
Evidência: docs/evidence/hub-outbox-arrival-worker-20261104.md.

### Confirmação da configuração recebida pelo ingest — candidato0.8.62

- [x] Reproduzir v41 aplicada sem report após204 condicional.
- [x] Solicitar confirmação própria na cadência normal, sem bloquear ingresso.
- [x] Repetir confirmação após falha503; preservar relay/duplicatas/auth.
- [x] Testes HTTP/race com fonte de preferências real.
- [x] Consolidar check completo/revisão e commit exclusivo.
- [ ] Autorizar publicação/piloto0.8.62 somente HUB84 e validar ACK do OFF.

0.8.61 foi publicada no piloto; esse fato não encerra PKG-33 nem credencial
inválida, histórico central ou confirmação v41. Ver docs/evidence/hub-outbox-config-confirmation-20261104.md.

### Amplificação de escrita do HUB — candidato 0.8.63

- [x] Medir deltas de I/O por processo e reproduzir amplificação em fixture.
- [x] Agrupar Push/ACK com persistência durável, limite transacional e opt-out somente HUB.
- [x] Testar race, falha concorrente, perda do processo, reabertura legada e HTTP de 64 relays.
- [x] Medir benchmark do store real e registrar limites de conclusão.
- [x] Publicar0.8.63 por autorização específica e confirmar somente HUB84; I/O caiu~19%, CPU ainda elevada; não encerra consumo.

Candidato local; CPU integral ainda sem profiler produtivo. Não fecha PKG-33/134, não modifica histórico, credenciais ou retenção. Evidência: docs/evidence/hub-durable-write-batching-local-20261104.md.

### Evidências do incidente

- Relay em `AGENT_MODE=relay` com `channel.connected=true`, `relay_uses_hub_url=true` e sem conexão direta com AIceberg.
- Relay gerando `collect buffered route=/v1/ingest/metrics` e `flushed` sem erro.
- Hub em `AGENT_MODE=hub` com `queue_items` crescendo e `flush_err` aumentando.
- Erro do Hub: `Post "https://api.aiceberg.com.br/v1/ingest/metrics": context deadline exceeded`.
- Banco sem `metrics` recente do relay afetado, apesar de `bootstrap`, `health`, `network_capture` e `inventory` recentes.

### Lotes propostos

1) **Configuração operacional do flush**
   - [x] adicionar variáveis/env e prefs remotas para timeout HTTP de ingestão (`INGEST_TIMEOUT_SEC` ou equivalente);
   - [x] tornar tamanho de batch da outbox configurável sem recompilar (`OUTBOX_FLUSH_BATCH` ou equivalente);
   - [x] permitir intervalo de flush configurável quando o Hub estiver sob backlog;
   - [x] expor a configuração efetiva no `/health`, no comando `inspect_runtime_config` e no payload de métricas do agente;
   - [x] manter defaults compatíveis com instalações existentes.
   - [x] limitar cada requisição serializada a 8 MiB e dividir automaticamente o grupo sem depender apenas da quantidade de envelopes.
   - [x] fracionar atomicamente envelopes individuais excessivos de `/v1/logs/raw` sem perder eventos nem metadados.

2) **ACK granular e isolamento por rota**
   - [x] alterar `FlushOutbox` para confirmar ACK dos grupos enviados com sucesso mesmo se outro grupo falhar;
   - [x] isolar falhas por `authHeader + endpoint`, evitando que timeout em `/v1/ingest/metrics` prenda `health`, `bootstrap`, `inventory` e outros endpoints;
   - [x] preservar envelopes não enviados para retry sem duplicar os já confirmados;
   - [x] registrar logs com `route`, `batch_size`, `acked`, `retained`, `duration_ms` e `err`;
   - [x] cobrir com testes unitários de sucesso parcial e falha por endpoint.

3) **Retry, backoff e descarte seguro**
   - [x] implementar retry com backoff por rota/autorização, evitando loop agressivo quando a API estiver lenta;
   - [x] respeitar respostas de backpressure da API quando disponíveis (`retry-after`, batch sugerido, rota degradada);
   - [x] classificar erro temporário, erro HTTP definitivo e envelope inválido;
   - [x] permitir descarte auditável de envelope irrecuperável sem bloquear a fila inteira;
   - [x] manter segurança: nunca descartar payload por timeout temporário.

4) **Priorização operacional em Hub**
   - [x] priorizar telemetria curta de saúde/fila/canal para manter diagnóstico em tempo real;
   - [x] evitar starvation de métricas recentes quando houver backlog antigo grande;
   - [x] avaliar filas lógicas por endpoint ou leitura balanceada da outbox preservando idempotência;
   - [x] garantir que Relay sem saída direta continue sem conectar ao AIceberg.

5) **Observabilidade local do backlog**
   - [x] expor no `/health` e `/metrics`: último flush com duração, batch confirmado, batch retido, rota com última falha, idade do item mais antigo e contadores por endpoint;
   - [x] incluir diagnóstico do backlog no comando remoto `inspect_runtime_config`;
   - [x] melhorar logs de `transport failed` com `status`, timeout, rota, tamanho do lote e próxima tentativa;
   - [x] documentar comandos de suporte para Hub/Relay.

6) **Homologação HUB/Relay sob carga**
   - [x] criar teste local/simulação com Hub recebendo relays e API lenta em `/v1/ingest/metrics`;
   - [x] validar que `health/bootstrap/inventory` continuam fluindo mesmo com timeout de `metrics`;
   - [x] validar que métricas recentes voltam a drenar após recuperação da API;
   - [ ] [bloqueado] medir crescimento/drenagem da fila em cenário com Agentless ativo;
   - [x] rodar `./check.sh` antes de gerar qualquer artefato.

7) **Publicação e rollout**
   - [x] publicar artefatos oficiais da nova versão antes de acionar update remoto;
   - [ ] [validacao] validar update em um Hub e um Relay de homologação;
   - [x] documentar rollback para versão anterior e parâmetros temporários de mitigação;
   - [x] registrar no web qual versão mínima do agente possui flush resiliente.

### Critérios de aceite

- [x] timeout de `/v1/ingest/metrics` não impede ACK de outros endpoints enviados com sucesso;
- [x] Hub não mantém crescimento indefinido da fila quando a API volta a responder;
- [x] `/health` mostra dados úteis de backlog e última falha sem depender de `journalctl`;
- [x] batch e timeout podem ser ajustados por configuração;
- [x] Relay continua enviando somente ao Hub em modo `relay`;
- [x] testes cobrem falha parcial, retry e ACK granular;
- [x] `./check.sh` passa antes de release.

### Fora de escopo inicial

- desligar coletas como solução definitiva;
- conexão direta obrigatória de `relay` com AIceberg;
- remover outbox persistente;
- trocar o protocolo HTTP de ingestão antes da correção de resiliência;
- alterar banco da Web a partir do repositório do agente.

---

## [PKG-73] Agente/SOC — taxonomia canônica de origem, campos SOC e roteamento seguro para PKG-54

**Status** — implementado e validado em 19/06/2026 como pacote coordenado com `aiceberg_web`.

**Objetivo** — emitir contrato SOC canônico em todos os logs relevantes do agente sem quebrar payload legado, reduzindo inferência frágil no PKG-54.

### Lotes concluídos

1) **Contrato canônico de origem SOC no agente**
   - [x] adicionar `aiceberg_transport`, `aiceberg_tool_origin`, `aiceberg_source_category`, `aiceberg_soc_source_type`, `aiceberg_soc_eligible`, `aiceberg_origin_confidence` e `aiceberg_route_reason`;
   - [x] manter compatibilidade com `transport`, `source_tool`, `source_category`, `level`, `severity`, `attributes` e `/v1/logs/raw`;
   - [x] cobrir `windows_eventlog`, `ad_security`, `linux_syslog`, `linux_auth`, `journald`, `graylog_gelf`, `application`, `container_log`, `kubernetes_log`, `otlp_log` e `unknown`;
   - [x] documentar contrato em `docs/pkg73_soc_log_origin.md`;
   - [x] validar payload legado pelo normalizador web.

2) **Campos SOC mínimos normalizados**
   - [x] promover campos seguros quando existirem: `event_code`, `vendor`, `product`, `src_ip`, `dst_ip`, `src_host`, `dst_host`, `username`, `process_name`, `command_line`, `file_hash`, `domain`, `url`, `action`, `rule_name`, `technique_id` e `alert_id`;
   - [x] manter `attributes` sanitizado como evidência reduzida;
   - [x] impedir promoção de senha, token, cookie, segredo e Authorization;
   - [x] validar app JSON, Graylog/GELF, Windows Security, Linux auth, Docker, Kubernetes e OTLP.

3) **Classificação segura por origem**
   - [x] classificar Windows `Security`/Security-Auditing como `ad_security`/`windows_security`;
   - [x] classificar Sysmon como SOC elegível;
   - [x] manter Windows `System`/`Application`, incluindo DistributedCOM `10028`, como observability/no;
   - [x] classificar Linux auth como `linux_security`;
   - [x] classificar app JSON, OTLP, container e Kubernetes como `conditional` por padrão;
   - [x] preservar Graylog/GELF e ferramenta real quando configurada/inferida;
   - [x] validar DCOM/RPC operacional sem promoção SOC.

4) **Configuração e override controlado**
   - [x] aceitar campos `aiceberg_*` em logs estruturados;
   - [x] aceitar Docker labels `aiceberg.ai/*` e `aiceberg.com/*`;
   - [x] aceitar Kubernetes annotations `aiceberg.ai/*` e `aiceberg.com/*`;
   - [x] manter `logs.processors`/`OSLOG_PROCESSORS_JSON` para remap/route/enrich sob as regras de assinatura/escopo do PKG-68;
   - [x] separar `configured`, `inferred` e `unknown`;
   - [x] divergência/ausência vira lacuna ou `conditional`, não SOC crítico automático.

5) **Consumo web e roteamento para PKG-54**
   - [x] entregue no `aiceberg_web` por `SocAgentLogOriginNormalizerService`;
   - [x] backend adiciona snapshot `AICEBERG_SOC_ORIGIN=` no conteúdo do `log_bruto` sem SQL novo;
   - [x] payload novo e legado são aceitos;
   - [x] PKG-54 pode extrair transporte, origem, categoria, tipo SOC, elegibilidade, confiança, motivo, campos SOC e lacunas.

6) **Evidência, homologação e pacote de release**
   - [x] evidência controlada em `docs/evidence/pkg73/soc-log-origin-20260619T210500Z/evidence.md`;
   - [x] testes focados do agente executados;
   - [x] teste focado web executado;
   - [x] documentação atualizada;
   - [x] fechamento depende de `./check.sh` nos dois repositórios.

### Critérios de aceite

- [x] todo log coberto pelo agente possui transporte, origem, categoria, elegibilidade SOC e confiança;
- [x] Graylog/GELF não mascara ferramenta real quando configurada/inferida;
- [x] Windows EventLog preserva campos essenciais e classifica canais corretamente;
- [x] logs de aplicação/observabilidade não entram como SOC crítico por padrão;
- [x] fontes security conhecidas carregam campos SOC mínimos quando disponíveis;
- [x] campos sensíveis não vazam em top-level SOC;
- [x] PKG-54 consegue consumir o contrato sem inferência frágil;
- [x] agentes/payloads antigos continuam compatíveis.

### Rollback

Desligar logs por `OSLOG_ENABLED=false`, remover overrides `aiceberg.*`, ignorar campos `aiceberg_*` no backend ou publicar versão anterior.

---

## [PKG-74] Agente/Web — descoberta automática de fontes de logs, aplicações e dependências locais

**Status** — implementado em código em 20/06/2026 como entrega coordenada com `aiceberg_web`. Testes focados e `./check.sh` executados; artefatos `0.8.13` gerados e publicados no web. Validação real parcial em produção atualizou seis agentes online do cliente InspectApp para `0.8.13`, consumiu `collect_now=log_source_discovery` e persistiu candidatos reais no web. Bundle controlado `docs/evidence/pkg74/discovery-controlled-20260620T180000Z` cobre troubleshooting de aplicação lenta com web/app/banco/rede, runtime, segurança, Kubernetes/OTLP controlados, redaction e lacuna de permissão. Desktop `72` está desligado e foi removido do bloqueio desta meta por decisão operacional do usuário; IIS/Kubernetes reais ficam como homologação por cliente quando existirem no ambiente alvo.

**Problema a resolver** — o agente já coleta fontes configuradas e emite taxonomia SOC, mas ainda não inventaria automaticamente tudo que pode ajudar Log/NOC/SOC/APM/troubleshooting no host: IIS, Nginx, Apache, Plesk, aplicações, bancos, filas, containers, Kubernetes, EventLog Security, Linux auth, serviços, portas e dependências. Sem isso, a web não consegue listar fontes candidatas para aprovação e a IA recebe contexto incompleto.

**Objetivo** — implementar descoberta local segura, bounded e read-only de fontes de log, aplicações e dependências, reportando candidatos `log_source_discovery_v1` para o web. O agente deve propor candidatos com evidência, confiança, severidade mínima, risco, volume e permissões, mas a coleta ampla depende de configuração aprovada/remota e assinada pela web.

**Reuso obrigatório** — coletores `oslogs`, `journald`, `containers`, `kubernetes`, `otlp`, `localchecks`, `networkcapture`, snapshot/bootstrap, configuração remota assinada do PKG-68, runtime de plugins/checks do PKG-66, integrações do PKG-71, contexto do PKG-72 e contrato SOC do PKG-73. Não criar runtime paralelo nem shell arbitrário.

**Fora do escopo** — varrer disco inteiro, coletar `debug`/`info` por padrão, ler segredos, executar comandos remotos arbitrários, ativar EDR/NDR/SOAR, alterar banco da web, burlar permissões do sistema operacional ou declarar causa raiz sem evidência.

### Lotes propostos

1) **Contrato e domínio de discovery**
   - [x] [exec] criar domínio/contrato `log_source_discovery_v1` com `schema_version`, `agent_id`, `asset_id`, `host`, `os`, `collected_at`, `scan_policy`, `capabilities`, `candidates[]`, `gaps[]` e `redaction_summary`;
   - [x] [exec] cada candidato deve expor `fingerprint`, `kind`, `product`, `service_name`, `process_name`, `port`, `listener`, `path`, `channel`, `unit`, `container`, `pod`, `namespace`, `runtime`, `version`, `confidence`, `evidence`, `recommended_category`, `useful_for`, `soc_source_type`, `soc_eligible`, `origin_confidence`, `min_severity`, `estimated_volume`, `usefulness_score`, `risk_score`, `permissions_required`, `redaction_policy`, `retention_hint`, `freshness`, `status` e `rollback_ref`;
   - [x] [exec] deduplicar candidatos por fingerprint estável;
   - [x] [validacao] validar serialização, compatibilidade de snapshot/bootstrap e payload parcial.

2) **Descoberta Linux**
   - [x] [exec] descobrir systemd units, processos, listeners e paths ligados a `nginx`, `apache/httpd`, `php-fpm`, `tomcat`, `java`, `node`, `python`, `dotnet`, `plesk`, `ssh`, `auth.log/secure`, `journald`, `syslog`, `postgres`, `mysql/mariadb`, `redis`, `rabbitmq`, `mongodb`, filas e aplicações comuns;
   - [x] [exec] mapear paths/canais conhecidos sem scan recursivo amplo, com allowlist de diretórios, limite de candidatos e permissões;
   - [x] [exec] redigir secrets de cmdline antes de evidência ou flare;
   - [x] [validacao] fixtures/testes e hosts reais cobriram Nginx/Apache/Plesk/app/banco/rede; permissão negada coberta por teste controlado `path_permission_denied`.

3) **Descoberta Windows**
   - [x] [exec] descobrir EventLog `System`, `Application`, `Security`, Sysmon quando existir, IIS/W3SVC, Windows Defender, processos .NET/Java/Node/Python e listeners;
   - [x] [exec] preferir EventLog/canal estruturado a arquivo bruto quando possível;
   - [x] [exec] detectar permissão ausente sem falhar o agente;
   - [x] [validacao] cobrir Windows Server real com EventLog Security/Defender/SQL Server e IIS/app log controlados no bundle PKG-74; Windows desktop `72` permanece pendente por estar offline.

4) **Containers, Kubernetes, OTLP e APM**
   - [x] [exec] descobrir Docker/containerd por socket/permissão, log JSON path e portas sem expor env sensível;
   - [x] [exec] descobrir Kubernetes por namespace/pod quando rodando em cluster;
   - [x] [exec] correlacionar OTLP configurado e runtime local quando existirem;
   - [x] [validacao] cobrir Docker/containerd real, Kubernetes/OTLP controlados, permissão negada controlada e rollback transacional de config na web.

5) **Scoring, severidade e proteção de volume**
   - [x] [exec] classificar candidatos como `log`, `noc`, `observability`, `soc` ou `conditional`, com `useful_for` para Log/NOC/SOC/troubleshooting;
   - [x] [exec] aplicar `min_severity=error` por padrão para logs;
   - [x] [exec] descartar localmente evento sem nível quando severidade mínima configurada exigir nível conhecido, salvo override explícito;
   - [x] [exec] estimar volume e impor limite de candidatos/bytes de evidência;
   - [x] [validacao] provar que segredo não vaza em evidência por teste unitário.

6) **Aplicação de configuração aprovada**
   - [x] [exec] receber configuração remota assinada/escopada com fontes aprovadas via payload `logs`;
   - [x] [exec] ativar coleta somente para candidatos aprovados, mantendo configs atuais compatíveis;
   - [x] [exec] registrar versão de config, origem, rollback e status de aplicação pelo fluxo existente;
   - [x] [validacao] aprovar fonte/rejeitar/ignorar e rollback transacional já validados na web; perda de rede, API indisponível, disco cheio/outbox, proxy, payload grande, rollback de update e CPU/mem permanecem cobertos pelas evidências operacionais PKG-69 reutilizadas como base de homologação do agente.

7) **Evidência e fechamento**
   - [x] [validacao] rodar testes focados por coletor e contrato;
   - [x] [validacao] rodar `./check.sh`;
   - [x] [validacao] gerar bundle controlado com Linux, Windows/IIS controlado, Docker/containerd, Kubernetes, OTLP/APM, redaction, permissão negada, app lenta, banco e rede; compatibilidade com agente legado é validada no web por payload ausente/parcial; proxy, disco cheio/outbox, payload grande, CPU/mem e rollback de update referenciam evidências operacionais PKG-69 já versionadas;
   - [x] [exec] atualizar docs, testes, decisões e release somente após validação completa.

### Critérios de aceite

- [x] o agente descobre fontes úteis sem configuração manual inicial e sem scan amplo;
- [x] cada candidato tem fingerprint estável, evidência sanitizada, confiança, severidade mínima, volume estimado, risco, utilidade e permissões;
- [x] IIS, Nginx, Apache, app, banco, fila, EventLog Security, Linux auth, Docker, Kubernetes e OTLP/APM são detectados quando presentes ou em fixture controlada equivalente; IIS/Kubernetes reais seguem como homologação por cliente quando existirem no ambiente;
- [x] fonte sem permissão ou sem nível vira lacuna, não falso positivo;
- [x] coleta padrão respeita `error` ou superior e não envia `info/debug` para IA;
- [x] configuração atual, ingestão, snapshots, outbox e auto-update não quebram nos testes e update real dos seis agentes online;
- [x] nenhum segredo aparece em payload, log local, snapshot, flare ou evidência.

### Rollback

Desligar discovery por flag local/remota, ignorar `log_source_discovery_v1` no web, remover aprovações de fontes, voltar às coletas configuradas atuais ou publicar versão anterior do agente.

---

## [PKG-75] Logs/IA — sinais do agente para triagem recorrente e deduplicação segura

**Status** — implementado por compatibilidade de contrato em 20/06/2026. A triagem principal fica no `aiceberg_web`; o agente preserva origem, nível/severidade e sinais estruturados necessários para o backend não depender de texto livre. Correção runtime `0.8.14` gerada como pacote instalável para impedir auto-ruído de conectividade em logs coletados. Correção `0.8.15` adiciona identidade assinada nos endpoints de self-heal/erro do worker para clientes com identidade estrita habilitada.

### Lotes propostos

1) **Campos necessários para o backend**
   - [x] [exec] manter `level/severity`, `source`, `file`, canal/caminho e origem SOC quando disponíveis em logs enviados pelo agente;
   - [x] [exec] manter descarte local de evento sem nível quando `OSLOG_MIN_SEVERITY=error` estiver ativo;
   - [x] [exec] enviar descoberta de fontes locais para que o web diferencie origem real, canal estruturado, aplicação, banco, fila e runtime;
   - [x] [validacao] cobrir contrato por `go test ./internal/platform/collectors/logdiscovery ./internal/bootstrap`.

2) **Compatibilidade e rollback**
   - [x] [exec] não alterar contrato legado de `/v1/logs/raw`;
   - [x] [exec] permitir desligar discovery por `LOG_DISCOVERY_ENABLED=false` ou configuração remota;
   - [x] [validacao] validar update real para a versão nova em Linux e Windows: agentes Linux `1`, `4`, `18`, `19`, `70` e Windows `71` chegaram em `0.8.13`; desktop `72` está desligado e será testado futuramente se necessário.

3) **Auto-ruído de conectividade do agente**
   - [x] [exec] rebaixar timeouts transitórios de `config`, `ping` e `ingest` para log operacional `INFO`/backoff, mantendo `ERROR` para falha permanente ou erro local real;
   - [x] [exec] gerar versão `0.8.14` para impedir que `/var/log/messages`/EventLog capturem timeout transitório do próprio AIceberg como incidente do cliente;
   - [x] [validacao] cobrir regressão com testes focados de `ConfigSync`, `PingBackend` e `FlushOutbox`.

4) **Identidade estrita em controle do agente**
   - [x] [exec] enviar `X-Agent-Identity` em `selfheal-commands`, `selfheal-report` e `error-report`;
   - [x] [exec] gerar versão `0.8.15` para evitar falso alerta de identidade obrigatória quando o token é válido;
   - [x] [validacao] cobrir regressão com teste focado de `AgentControlClient`.

---

## [PKG-76] Agente — readiness EDR/NDR seguro

**Status** — implementado e validado em fixture/contrato controlado em 21/06/2026. Homologação real com CrowdStrike/Darktrace depende de fornecedor ativo no ambiente do cliente e não gera declaração ampla de compatibilidade sem evidência objetiva.

### Lotes propostos

1) **Modo seguro e limites**
   - [x] [exec] adicionar `EDR_SAFE`/`AICEBERG_EDR_SAFE` e `EDR_SAFE_PROFILE`;
   - [x] [exec] aplicar defaults conservadores para logs `error+`, discovery bounded, containers, Kubernetes, OTLP e checks locais;
   - [x] [exec] preservar métricas essenciais, inventário mínimo, logs úteis e auto-update seguro;
   - [x] [validacao] cobrir com `go test ./internal/common/config ./internal/domain/usecase`.

2) **Manifesto runtime**
   - [x] [exec] emitir `edr_ndr_readiness` no snapshot runtime com modo, política, manifesto, módulos, allowlist, lacunas e validação;
   - [x] [exec] bloquear shell remoto, ação destrutiva, execução arbitrária e claim de fornecedor sem evidência;
   - [x] [validacao] cobrir com `go test ./internal/bootstrap`.

3) **Coleta e compatibilidade**
   - [x] [exec] manter EventLog/journald/log discovery preferenciais e limitar arquivo bruto quando possível;
   - [x] [exec] manter compatibilidade com instalações atuais e configuração remota;
   - [x] [validacao] cobrir com `go test ./internal/platform/collectors/logdiscovery ./internal/platform/collectors/oslogs`.

4) **Fechamento**
   - [x] [validacao] rodar `go test ./internal/bootstrap ./internal/common/config ./internal/domain/usecase ./internal/platform/collectors/logdiscovery ./internal/platform/collectors/oslogs`;
   - [x] [exec] gerar versão instalável após `./check.sh`;
   - [x] [validacao] fechamento coordenado com o web para exibir readiness e bloquear auto-ruído interno em análise SOC.

### Rollback

Desativar `EDR_SAFE`, voltar ao perfil padrão, pausar módulos sensíveis por configuração remota assinada ou publicar a versão anterior do agente. Remover allowlist específica aplicada em fornecedor se ela tiver sido usada apenas para teste.

---

## Modelo de trabalho IA - 2026-05-22

Projeto: `aiceberg_agent`. Stack: Go agent/CLI, gopsutil, NTP, SNMP. Este arquivo segue `PATHS.toml` e deve ser mantido atualizado ao iniciar ou alterar o projeto.

## Quando criar pacote

Criar pacote quando houver varias demandas relacionadas, risco transversal, mudanca em mais de um modulo ou necessidade de dividir entrega em lotes. Um lote pode resolver uma ou mais demandas, mas o pacote so fecha apos check completo e review de fechamento.


## [PKG-110] Agentless — capacidade de coleta e correção de OIDs

Status: implementação integrada na `main`; `0.8.50` e `0.8.51` foram publicadas. O piloto comprovou o bloqueio do lançador privilegiado do agente antigo e deixou cooldown local para o alvo `0.8.51`; a `0.8.52` preserva a correção e cria um novo alvo para o piloto restrito. Escopo coordenado com o Web, cliente 2/HUB 10. Não ampliar automaticamente outros agentes.

- [x] Implementar lote com até oito destinos em paralelo e ordem sequencial por endereço, cancelamento e propagação de falha de persistência.
- [x] Serializar lotes e flushes concorrentes; drenar no máximo oito lotes por flush, preservando observações quando o envio falha.
- [x] Suspender novos polls de rotina quando a fila atingir oito lotes; não apagar dados nem transformar falha de gravação em sucesso.
- [x] Normalizar ponto inicial no casamento de OIDs de GET customizado.
- [x] Integrar e commitar separadamente a correção de OID (`dd2089f`) e a capacidade controlada (`f1b2f00`).
- [x] Gerar a versão `0.8.50` assinada e publicar os artefatos oficiais.
- [x] Gerar e publicar a versão corretiva `0.8.51` assinada; o piloto automático permaneceu bloqueado no agente antigo.
- [ ] Gerar a versão `0.8.52` assinada para repetir o piloto sem o cooldown local associado ao alvo `0.8.51`.
- [ ] Atualizar somente o HUB piloto, validar versão/serviço, medir regularidade e só então ampliar Web e inventário.

Rollback: pacote anterior assinado, política de despacho Web vazia e configuração anterior do HUB. Preservar outbox e histórico. Sem alteração de schema ou rotina nova de limpeza.

## Correção operacional de coleta SBC via HUB84 — 16/10/2026

- [x] Diagnosticar fila cheia do HUB e confirmação incorreta de persistência no handler relay.
- [x] Corrigir ACK em falha/parcial e disponibilizar diagnóstico finito e sanitizado da outbox pelo canal existente.
- [x] Gate completo, pacote0.8.55 assinado e aplicação somente ao HUB84 (config25/version_confirmed).
- [ ] Comprovar escoamento e regularidade das medições do SBC674, sem interferir no Runtime/GOS.

- [x] Diagnóstico produtivo pela0.8.55 confirmou fila cheia apesar de ACKs válidos e gargalo no lote serial50.
- [x] Implementar0.8.56: capacidade HUB400/4 limitada, identidade/retry isolados e callback de configuração serializado.
- [x] Publicar0.8.56 assinada e aplicar somente ao HUB84 (config26/version_confirmed18:04:04UTC); commit468cb44, Web3a74351e, esteira38944283175 SUCCESS.
- [ ] Comprovar coleta SBC regular e janela de oito minutos completa.

- [x] Medir capacidade após0.8.56: fila volta a200MB com206 relays ativos; defaults400/4 insuficientes.
- [x] Publicar0.8.57 assinada com capacidade800/8 restrita ao HUB84; config27/version_confirmed às18:26:37UTC. Fontef2ed837 enviada; hash público Linux conferido.
- [ ] Concluir escoamento sustentado e avaliação natural da janela: SBC recebeu memória75% às18:30:48UTC, atraso de entrega246,6s. Média em lote depende da correção PHP/Go coordenada com a frente GOS.

## Central de atualização — pré-verificação local do updater (16/10/2026)

- [x] Acrescentar ao comando existente `inspect_runtime_config` o pedido explícito
  `payload.update_readiness=true`, sem novos comandos de shell ou alteração de configuração.
- [x] Retornar contrato versionado, horário UTC, SO/arquitetura, versão/configuração e
  verificações booleanas de updater, scripts, serviço, privilégios e pendência local.
- [x] Preservar estado pendente; não baixar pacote, aplicar, reiniciar ou coletar flare.
- [ ] Concluir gate completo, publicar nova versão assinada e validar em hosts piloto.
- [x] Implementar instalador, daemon launchd e updater macOS com SHA256 obrigatório, health local por versão e rollback; testes locais isolados, sem instalação de serviço.
- [x] Integrar dispatcher/lotes no Web; resultado sem schema novo exige preparação. Candidato Web commitado, ainda não publicado.
- [ ] Validar instalação e atualização real em macOS piloto; repetir gates e publicar artefatos assinados da nova versão antes de liberar a central.

Riscos remanescentes de apply (bloqueio do binário, EDR/antivírus, políticas de sistema)
continuam explícitos; pré-verificação não substitui confirmação pós-reinício. Runtime/GOS
não são alterados nesta entrega.

### Redução adicional de CPU/I/O — candidato0.8.64, PKG-33

- [x] Medir escrita sustentada em0.8.63 e confirmar aplicação/OFF no piloto autorizado.
- [x] Reproduzir custo da freelist em chegadas espaçadas no BoltStore real, inclusive Linux.
- [x] Omitir somente freelist reconstruível no HUB, com fsync de dados/meta e opt-out independente.
- [x] Evitar serialização de métricas no normalizador exclusivo de logs, preservando limite de envio.
- [x] Testar upgrade, kill, reabertura legada, isolamento por modo, opt-out e regressões com race.
- [x] Consolidar review/check e candidato assinado0.8.64; cinco pacotes/hashes/Ed25519 verificados.
- [x] Publicar0.8.64 autorizada somenteHUB84; hash/OFF53 confirmados, escrita-78%, CPU44,87% e picos no relay permanecem.

Nenhuma limpeza, replay ou mudança de credencial. CPU integral do HUB ainda não comprovada por profiler instalado. Ver docs/evidence/hub-freelist-and-normalization-local-20261104.md.

### CPU do HUB e envio do relay — candidato0.8.65, PKG-33

- [x] Diferenciar atraso envelope→API de localização do atraso; inventariar timer síncrono do relay.
- [x] Eliminar materialização redundante de Body no listener/Bolt do HUB, com flag de rollback.
- [x] Reutilizar worker independente no relay; manter configuração no loop principal, ACK, cooldown e pruning.
- [x] Adicionar carimbo de recepção no HUB e contador de CPU cumulativa, sem tabela ou log novo.
- [x] Testar pipeline HTTP/Bolt real, coletor bloqueado, falha503, reabertura, split e limites por modo com race.
- [x] Medir leitura/forwarding estruturado em macOS e Linux sintético; gain local separado do piloto.
- [x] Consolidar check/review/commit exclusivo da correção local.
- [ ] Gerar/verificar cinco pacotes assinados antes da autorização de publicação.
- [ ] Autorizar novo push/publicação e piloto escalonado:HUB84 e somenteRelay124.
- [ ] Medir saltos/CPU por janela e comprovar redução de picos antes de encerrar.

Preservar histórico, filas,27confirmações incertas e credencial de origem. Não fecha PKG-33/134. Evidência: docs/evidence/hub-cpu-relay-arrival-local-20261104.md.


## [PKG-49] Entrega rápida Agentless — delta coordenado Agent/Web

**Implementação e publicação100% — 06/11/2026(local).** Escopo principal e gates externos no
`aiceberg_web/DEMANDAS.md`, PKG-49. Usuário autorizou implementar/commitar na main
deste repositório em06/11, sem push/publicação. Não altera autorização de outros pacotes.

- [x] L08: `delivery_timing/agentless_delivery_v1` com RFC3339 UTC por tentativa,
  coleta original/IDs/dedupe preservados. Consumidor Web distingue medição local,
  estimativa entre relógios e conclusão do save antes do commit.
- [x] L09: política opt-in `AGENTLESS_DELIVERY_PRIORITY=false`, seleção justa de
  cabeças por check/segmento/coleta em memória/Bolt, sem mudar schema do arquivo.
  Três slots rápidos recentes/um mais antigo, sem ultrapassar parcial/final.
- [x] Recusa de payload/resultado parcial isolada apenas com dedupe, com todos os
  splits dentro das oito tentativas existentes. ACK vazio/inválido/divergente não
  remove dados; falha global/auth/rate limit/transporte mantém interrupção/cadência.
- [x] L10: HTTP/Bolt reais controlados, reinício, memória/Bolt equivalentes,
  prioridade/justiça/ordem/legado, cancelamento, resposta parcial e rollback por
  flag. `-race`, gate `./check.sh`, revisão e commits próprios coordenados no Web.

- [x] Publicação:fonte076b0df limpa/sincronizada, versão0.8.66 assinada nas cinco
  plataformas, sete hashes públicos/cinco Range206 e Webf0ce186e/success.
  Registro em docs/evidence/pkg49-production-20261106.md.
- [ ] Ativação:autorizar HUB alvo, atualizar/confirmar versão instalada e habilitar
  prioridade gradualmente. Publicação no catálogo não equivale a instalação.
- [ ] Homologação de baseline/depois, relógios, carga, backlog, retry/falhas/tela e
  rollback sem excluir registros.
- [ ] Comprovar metrics_fast em1–2min após coleta com janela/amostra reais e
  resultado do atraso histórico. Não há prova produtiva neste ciclo.

Evidência: `docs/evidence/pkg49-local-20261106.md`. Sem novo coletor, scheduler,
fila, tuning em frota, limpeza ou descarte. Push/distribuição autorizados nesta
publicação; sem update/ativação de HUB/Relay. Homologação e SLA não comprovados.

### PKG-134 — preparação da publicação 0.8.67 (08/11/2026)

Versão exclusiva para os diagnósticos e recuperação de aplicação de configuração.
Publicação e instalação piloto autorizadas pelo proprietário após o fechamento técnico.
Artefatos devem ser assinados e publicados somente depois do Web e da API compatíveis;
piloto restrito, sem injetar falhas de disco/permissão nos hosts e sem ampliar a frota.
Homologação operacional completa e medição continuam separadas.

### PKG-134 — publicação0.8.67 e piloto instalado (08/11/2026)

Fonte do binário:a1095ff4e84e83faede9dd1b5b549c8614cf9275, árvore limpa.
Cinco compactados, SHA256SUMS e manifesto Ed25519 verificados e publicados em
https://cliente.aiceberg.com.br/downloads/agent/0.8.67/. Web92c3ef7c, esteira
42496278053 SUCCESS; hashes públicos iguais aos locais, chave privada local.
Lote oficial3/cliente1/concorrência1: agentes4(HUB/Linux) e71(Windows direto),
ambos0.8.67/version_confirmed/configuração applied/sem update pendente. As11
condições de readiness passaram em cada host; não houve envio a outros agentes.
Linux healthok e hash instalado igual ao pacote; recibos reais71838637/71839414.
Caminho público API-006 permanece PHP compatível; Go não foi ativado/mudado para
o piloto. Homologação de falhas/recuperação/rollback e medição de valor abertas.

Acompanhamento registrou recorrência do crash do MySQL gerenciado e maintenance
FATAL; instalação inicial concluída não prova estabilidade do backend. Não
ampliar piloto com base nesse recibo. Detalhes/rollback e evidência integrada
em aiceberg_web:docs/evidence/pkg134-production-20261108.md. Este registro
documental não recompila nem substitui os artefatos públicos imutáveis0.8.67.

### PKG-134 — correção da cadeia de build0.8.68 (08/11/2026)

A verificação do artefato Linux0.8.67 identificou Go1.25.4 e x/net0.44.0
sinalizados no scanner de componentes. Não confundir presença de componente
com exploração comprovada; a versão é substituída por novo release imutável.
Fixar Go1.26.10/min1.26 e x/net0.62.0/requisitos transitivos mínimos; verificar
binários finais com Trivy rootfs (scan filesystem sem gobinary não é evidência),
checks completos, builds/plataformas, assinatura e fonte limpa antes de publicar.
Preservar0.8.67 e rollback/configuração/outbox; piloto restrito aos mesmos4/71,
sem ampliar a frota. Homologação e causa dos crashes do MySQL continuam abertas.

Validação da correção0.8.68: race dos12pacotes de configuração/estado/updater/
HUB/health/bootstrap/coletores afetados PASS; interop serializador Go→PHP real
PASS. Lint v2.10.1 com defaults v1 migrados e oito equivalências mecânicas PASS,
sem ignorar novos QF. ./check.sh PASS. Cinco builds de laboratório Go1.26.10
reconhecidos por Trivy rootfs, zero HIGH/CRITICAL nos cinco OS/arch. Candidatos
sujos de laboratório não serão distribuídos; reconstruir/assinar da fonte limpa
com revisão/VCS/manifesto conferidos após este commit. Sem SQL ou expansão.

### PKG-134 — piloto final0.8.68 confirmado (08/11/2026)

Artefatos finais da fontede207d9c882599b4277771a3b935ff24c1c9d714: Go1.26.10,
vcs.modified=false nos cinco binários, scanner rootfs5alvos/zero HIGH-CRITICAL
e5assinaturas válidas. Sete arquivos0.8.68 públicos200/SHA iguais aos locais;
Web33451857/esteira42503432242 SUCCESS.0.8.67 preservada, sem ampliar seu piloto.
Lote4/cliente1/concorrência1 completed: agente4Linux HUB0.8.68/applied88 e
71Windows direto0.8.68/applied37, version_confirmed e sem update pendente.
Inspector novo/11condições aprovadas em cada host; recibos reais71912216 e
71912295 após reinício. Hash Linux instalado igual ao pacote e healthok.

Maintenance retomado somente após conexão nova/Uptime371s e fila vazia para a
correção autorizada; isso não resolve o crash recorrente do banco. Nenhuma
mutação de fila/histórico ou bypass de assinatura/contingência de publicação.
Homologação de falhas/Relay/rollback/aceite e medição permanecem separadas.
Este recibo não recompila ou altera o release público0.8.68. Evidência integrada
em aiceberg_web/docs/evidence/pkg134-production-20261108.md.

## [PKG-80] Suporte remoto — componente Agent — 17/11/2026

Implementação local L08–L10 do backlog Web concluída; main autorizada.
Check integral PASS, inclusive lint/vet/testes e gates oficiais. Candidata0.8.69,
ainda sem distribuição/instalação. Publicação/pilotos reais permanecem separados.

- [x] Produtor de journal Linux/Event Log Windows ligado ao bootstrap existente.
- [x]16KiB/fonte,5s,100registros; redação de linhas sensíveis, sem shell/path remoto.
- [x] Callback/ferramenta ausente, fonte vazia, permissão negada e cancelamento
  resultam em falha diagnosticável, sem sucesso por snapshot isolado.
- [x] Regressões com-race no bootstrap, coletor e executor, com subprocessos
  controlados; o teste de seleção de ferramentas usa fixture POSIX e não prova
  execução/permissão no Windows real.
- [ ] Artefatos assinados, publicação e piloto SADA Linux/Windows com recibos reais.

Contrato e evidências oficiais no Web:docs/architecture/agent-support.md e
 docs/evidence/pkg80-local-20261117.md. Sem alteração de outbox, token ou dados.

Piloto18/11: HUB84 confirmou0.8.69, mas journal negou permissão. Correção0.8.70
mantém a falha da fonte SO e usa fallback somente de logs reais deste processo,
redigidos antes da retenção em memória:16KiB/100entradas/1h, sem arquivo/tabela,
sem ampliar journal/ACL ou aceitar path remoto. Fonte distinta e degraded=true;
callback ausente, cancelamento, plataforma não suportada e buffer vazio continuam
falhando. Piloto real e confirmação do Windows ainda abertos.

### Revisão pré-piloto do diagnóstico —20/11/2026

rc.3 assinada e disponibilizada, sem instalação. Revisão do gosnmp1.38.0 encontrou
aceitação de community divergente e requestID0 pelo decoder. rc.4 acrescenta
validação explícita apenas no GET básico; erros tipados sem segredo, mantendo
counters UDP e sem qualificar resposta incompatível. Fixtures UDP cobrem ambos.
A rc.3 permanece imutável; usar rc.4 no piloto do HUB84 após gates e assinatura.

### PKG-115 — fluxo ativo SNMP,21/11/2026

RunJob/partiais agora usam definição validada para projetar CPU/memória da mesma
coleta e enviam monitoring_samples_v1 como metadado opcional via Bolt/HTTP.
Sem mudar payload/IDs/dedupe/ACK ou promover diagnóstico a rotina. Qualificação
UDPv1 real local→Bolt reaberto→HTTP503/retry→PHP/MySQL→Go passou; não é sensor
físico. Candidata0.8.72-rc.2; assinatura/publicação/update são gates separados.
Demais fontes/pipelines/modelo universal continuam abertos. Contrato Web:
docs/contracts/monitoring-execution-v1.md.


### PKG-115 — item1/14: TCP até a persistência,22/11/2026

Segundo coletor canônico: RunJob TCP projeta a latência dedicada da conexão já
realizada, após validar definição/escopo, sem probe adicional ou mudança no payload
legado. Uma amostra integer/milliseconds; zero real válido, falha/parcial/inválido
unknown/null. Estado não segmentado legado é final só para a projeção. Bolt/HTTP
preservam IDs/dedupe/retry/ACK. Servidor PHP reconstrói por check autenticado,
sem confiar em metadado recebido. MySQL nativo e regressão SNMP/consumidor Go
qualificados, fixture própria removida. Contrato oficial Web:
`docs/contracts/monitoring-tcp-execution-v1.md`. Checklist14 permanece no backlog
Web; item1 implementado/qualificado, demais13 continuam abertos. Não fecha PKG-115
nem autoriza declarar nova candidata instalada. Sem nova versão/artefato neste lote.


### PKG-115 — item2/14: registros tipados e estados,22/11/2026

Contrato monitoring.records.v1 implementado no domínio puro, coordenado com Web
PKG-115/Runtime19. Tipos number/integer/boolean/text/event, seis estados/reasons,
null explícito, cobertura complete/partial, identidade/escopo/revisão e ocorrência
de evento estável. Decode/encode estritos com teto de bytes/profundidade/quantidade,
duplicatas/UTF8/surrogates/número inválido recusados. Evolução não troca tipo/unidade
na mesma série. evaluated_at_ms preserva qualidade original; RecordFreshness calcula
idade atual sem apagar história ou promover unknown para ok.110vetores semânticos,
12wire/8evolução compartilhados PHP/Go, race/fuzz/roundtrip/limites. Contrato oficial
Web docs/contracts/monitoring-records-v1.md. Sem transporte/collector emitindo este
formato, alteração de payload SNMP/TCP, nova versão/artefato/update ou SQL.
Item2 implementado/qualificado;12itens3–14 permanecem na checklist oficial Web.


### PKG-115 — coordenação do item3/14 Web115,22/11/2026

Negociação pura monitoring.records.v1 para2..8participantes, fontes/tipos por
interseção, limites menores e trânsito JSON opaco obrigatório. Codec limitado
antes do parser, sem coerção/fallback/ativação.86vetores compartilhados PHP/Go,
matriz9fontes×5tipos e Relay/HUB/Bolt/HTTP reais no Agent, com falha/reabertura/
retry/resposta perdida e consumidor PHP. Entrada gzip no HUB não suportada:
recusa/retencão qualificadas, sem mudar opções/defaults. Contrato oficial Web:
docs/contracts/monitoring-records-transport-v1.md. Item3 de desenvolvimento
qualificado; itens4–14 e aceite/publicação instalados continuam separados.
Não fecha o pacote e não altera coletores/filas/configurações de produção.


### PKG-115 — configuração composta, item4/14 Web115,22/11/2026

Contrato inerte monitoring.configuration.v1: definições canônicas completas,
origem/pins e vínculos explícitos de checks; parâmetros tipados já resolvidos.
Go Runtime usa o compositor existente; Agent/PHP validam sem recompor ou executar.
82vetores compartilhados,45combinações fonte/tipo, schema, referências/limites/
evolução e9catálogos PHP cifrados/CAS/restaurados. Editor Web qualificado em
1280/390px; aplicação legada422 sem escrita. Contrato canônico Web:
docs/contracts/monitoring-configuration-v1.md. Item4 de desenvolvimento concluído;
5–14 e instalação/aceite permanecem separados. Sem pipeline/coleta nova, SQL,
publicação/update, branch/worktree nova ou fechamento do pacote.


### Pipeline básico — item5/14,22/11/2026

Núcleo Go executável versionado/tipado com identity/scale/offset, plano imutável
fixado por hash exato/revisão/escopo e configuração compilada autorizada. Estágios
locais explícitos de entrada/resultado evitam reaplicação acidental; somente batch
final validado segue contrato existente. Orçamentos de etapas/operações/bytes/tempo,
inteiros exatos e falhas unknown/null sem zero fictício; erros de configuração,
limites ou cancelamento retornam sem saída parcial.76vetores comuns PHP/Agent/
Runtime, schema, race, concorrência, replay/recompilação, compositor real e TCP
loopback→pipeline→PHP. Sem SQL, UI, collector/scheduler produtivo, release/update,
configuração privada ou teste físico SADA. Contrato canônico Web:
docs/contracts/monitoring-pipeline-v1.md. Item5 de desenvolvimento qualificado;
itens6–14 abertos, pacote115/contraparte19 não encerrados.

## Cobertura de armazenamento — contraparte WebPKG-143,23/11/2026

- [x] Código local e fixtures para pressão passiva/espera MegaRAID, sem SMART, sem IA e sem alterar branch.
- [x] Paridade PHP/Go e casos de ausência/stall/fuso/isolamento com sink no-send.
- [ ] Check completo/revisão e publicação coordenada, sem incorporar modificações SNMP concorrentes.
- [ ] Prova instalada no agente70/cliente1, configuração/hash aplicado, persistência/painel/alerta e rollback.
Não encerrado; nenhum push, release/tag, atualização de agente ou notificação produtiva deste incremento.

### Pipeline de monitoramento — item 6, 23/11/2026

O executor Go do Agent e Runtime passa a validar faixa ou comprimento, extrair
uma captura por regex e selecionar valores tipados por JSONPath singular.
PHP verifica a estrutura e valida o batch de saída. A compilação Go autoriza
a sintaxe e o custo do regex antes da execução. Os vetores compartilhados
cobrem a ordem das etapas, limites, tipos, valores ausentes e falhas explícitas.
A entrega local mantém o contrato de entrada e saída do item 5. A adoção requer
confirmar que todos os executores da configuração suportam as novas operações.
Em rollback, restaurar primeiro a revisão compatível ou desativar a atribuição.
Ver contrato Web em docs/contracts/monitoring-pipeline-v1.md. Itens 7–14 abertos.


### Pipeline de monitoramento — item 7

XMLPath singular no Agente e Runtime, com seleção por filho direto, posição
e atributo final. Valida documento completo, limites, cardinalidade e tipo
antes de emitir resultado. XML rejeitado, ambiguidade, ausência e estouro de
limite são distinguíveis sem expor conteúdo. PHP valida o caminho e consome o
batch final. Ver contrato Web em docs/contracts/monitoring-pipeline-v1.md.
A adoção produtiva depende de executores compatíveis no item 14.


### Pipeline de monitoramento — item 8

Delta para gauge e contador, com base inicial explícita, reset e proteção
de faixa. Throttle por igualdade tipada e janela desde a última emissão.
Checkpoint bbolt por vínculo com recibos atômicos, recuperação após
reinício e recusa de observações conflitantes. Lote suprimido por inteiro
não gera batch. Ver testes de transação, reentrega e PHP no contrato Web
docs/contracts/monitoring-pipeline-v1.md. Ativação e gestão do ciclo de vida
dos checkpoints ficam no item 14.


### Pipeline de monitoramento — item 9

O Agente e Runtime executam os itens dependentes do modelo validado, com
ordem topológica e uma única passagem por nó a partir de um snapshot de
coleta. Tipos, unidades, frescor e indisponibilidade são propagados por ramo.
Delta e throttle usam checkpoint atômico do grafo. Qualificação local cobre
fan-out TCP real com uma conexão, resultado consumível por PHP e reentrega
do grafo após reabertura bbolt. Adoção em produção no item 14. Ver
docs/contracts/monitoring-pipeline-v1.md no Web.


### Pipeline de monitoramento — item 10

Itens calculados executam fórmulas numéricas tipadas no DAG, com orçamento e
validação estática de dependências, operadores e unidades. Lacunas, erros
numéricos e frescor permanecem explícitos. Identidade do resultado calculado
usa o ciclo, e o DAG preserva a idade da origem mais antiga para a cadeia.
O compositor Go preserva a declaração da fórmula, e PHP valida a configuração
e consome o lote. Os testes locais cobrem fórmulas encadeadas, múltiplas
origens, falhas isoladas e combinação com checkpoint. Contrato Web:
docs/contracts/monitoring-pipeline-v1.md. Adoção operacional no item 14.


### Pipeline de monitoramento — item 11

Etapa script WebAssembly com ABI para valores JSON tipados, no Agente e
Runtime. O perfil sem imports restringe memória, instruções, concorrência
e prazo. Tempo/capacidade abortam sem commit parcial. Falhas do valor são
explícitas e isoladas no ramo. PHP valida o envelope e o resultado. Os
testes locais cobrem rejeição de recursos proibidos, memória nova por
chamada, tipo, trap, orçamento, fan-out e reentrega. Ver o contrato Web
docs/contracts/monitoring-pipeline-v1.md. Adoção operacional no item 14.

Este commit inclui o núcleo Go e os checkpoints locais. A projeção do coletor e a integração no executor continuam em validação separada.
