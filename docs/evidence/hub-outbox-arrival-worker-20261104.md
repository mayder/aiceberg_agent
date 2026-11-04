# HUB: envio por chegada em worker dedicado — 04/11/2026

## Pedido e comportamento anterior

O usuário pediu que dados recebidos dos agentes fossem enviados assim que
chegassem, em thread separada. NewHandler já recebe HTTP concorrente, mas só
adicionava à outbox; envio/pruning rodavam no mesmo select dos coletores.
O timer normal é15s;610fead acrescentou retomada em1s após progresso/backlog.
Esse commit anterior é local, sem push ou rollout, e permanece na mesma main.

## Implementação

- Append sinaliza após Push bem-sucedido, preservando persistência/aceite.
  Canal de 1 posição agrupa rajadas, sem bloquear o produtor ou criar goroutine por item.
- Somente HUB com flag ASYNC habilitada usa worker dedicado. Timer inline é nil nesse
  modo; existe apenas um proprietário dos dois flushes. Cada flush conserva
  até 8 grupos paralelos,janela de 8s,8MiB,auth/identidade/rota eACK seguro.
  O worker é iniciado antes de abrir o listener para osRelays, inclusive no startup.
- Sinal durante envio fica pendente; worker inicia outra passagem após terminar.
  Startup recupera fila persistida e fallback periódico mantém retries quando
  não chega dado novo. Catch-up pós progresso continua como melhoria anterior.
- Em falha total sem ACK, novos sinais respeitam um guard local de 1s para evitar
  busy-loop de leitura. Backoff/cooldown HTTP existentes permanecem independentes.
- Pruning mantém a cadência normal; nenhuma limpeza, replay, alteração de idade,
  tamanho ou máximo por agente. MemStore/fallback preexistente não foi alterado:
  piloto exige comprovar o Bolt efetivamente ativo, sem prometer durabilidade em RAM.
  Dados recebidos dos Relays usam a outbox principal; a fila de logs locais do
  próprio HUB continua em memória, como antes, e não ganha durabilidade no restart.
- Dados/config/contadores compartilhados têm fronteiras explícitas: métricas
  atômicas, outbox/pendingconfig concorrentes; own config recebida no envio é
  aplicada pelo loop principal ao lado de config-sync, sem introduzir escrita de
  prefs pelo novo worker. A fila de configuração é limitada/cancelável, sem log de conteúdo.
- Run cancela seu contexto filho ao retornar, sinaliza listener e aguarda worker.
  ListenerHTTP respeita cancelamento; requisição de envio em curso pode terminar
  até o timeout configurado. Não enviar novos grupos após cancelamento.

Nova responsabilidade separada em outbox_flush_cycle.go/outbox_flush_worker.go
e outbox_ingest_config.go; app.go perde o bloco duplicado dos dois flushes.
Sem motor de alertas, coleta extra, shell remoto, alteração SQL ou storage novo.
Observabilidade utiliza flags/contadores/logs existentes; nenhuma tabela/arquivo
de aplicação ou retenção paralela. Volume de logs/tráfego pode crescer sob carga,
então o piloto acompanha I/O,rotação,journal/API junto da latência de entrega.

## Testes locais executados

1. Contrato do repositório: notificação observa dado já gravado; falha não acorda
   worker e ACK não gera notificação de chegada.
2. Worker:1.000 sinais ocupam1slot, ciclos não sobrepõem; wake durante envio,
   fallback e cancelamento inclusive em no-progress guard passam com -race.
3. Relay→HTTPHUB→Bolt→worker→HTTPAPI: timer configurado em 1 hora. Primeiro Relay
   recebe aceite enquanto API está bloqueada. Worker foi acordado pela chegada;
   outbox conserva o envelope até ACK. Shutdown espera a requisição em curso,
   remove só o confirmado e preserva dois pendentes após reabrir Bolt.
4. Rajada de 64 Relays HTTP concorrentes: aceite de todos enquanto API bloqueada;
   depois dreno completo, sem duplicação na fixture,8 requisições simultâneas no
   máximo,auth/rotas corretas e sem pruning por chegada.
5. Config própria vai para o loop principal; relay config segue sua fronteira;
   cancelamento desobstrui fila cheia. Flagsdefault/opt-out/inspector conferidas.
6. Mutação controlada que retira o wake reproduziu ausência de envio no teste
   com timer de 1 hora; arquivo restaurado e casos com -race passaram. Persistência503
   conserva os dados no Relay nos contratos anteriores reexecutados.

Fixtures próprias em loopback/arquivos temporários, sem tokens/dados produtivos.
Não é prova de SLA/capacidade,Agentless em produção,Windows/macOS físico ou
entrega exatamente uma vez em falha de rede; retry/idempotência anteriores
continuam necessários. `./check.sh` passou com validações configuradas/lint/vet/
suiteGo/self-tests. Cross-build Linux/amd64,Windows/amd64,Darwin/arm64 passou;
binários só de teste em/tmp, sem manifesto/assinatura/pacote oficial. O hook final
revalida o diff consolidado, incluindo a ordem de inicialização do worker.

## Rollback e entrega

HUB_OUTBOX_ASYNC_ENABLED=false volta ao modo inline 610fead; catch-up pode ser
desabilitado independentemente. Reinício/update precisam de autorização e
procedimento normal; preservar configuração e outbox. Nenhuma versão foi
alterada e nenhum pacote oficial/push/update foi feito nesta etapa.

Publicação/piloto posteriores precisam medir ACK/idade collection→received,
Agentless/coletas/relays,carga API/CPU/I/O e retomada após indisponibilidade.
Estágio preexistente do PKG-134 em DECISOES.md/demandas.md é preservado fora do
commit desta evolução do PKG-33.
