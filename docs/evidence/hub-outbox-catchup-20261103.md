# Retomada da outbox do HUB — 03/11/2026

## Evidência e limite da conclusão

Incidente original: disco da aplicação cheio, errosHTTP500 e dados cerca de18h
atrasados. Correções Web/storage/índice/auth foram publicadas separadamente.
Inspector114/HUB84 às23:32:38SP:6.513itens/149.611.947bytes,800porlote,
concorrência8,ciclo8.227ms,327retidos,last_error_kindhttp_401. Às23:43,última
coleta recebida ainda tinha203s de atraso e5.847itens. Observabilidade voltou
paraOFF/config38 comACK. Fontes/evidência operacional permanecem no Web/privado.

Fonte do agente: dispatcher tem janela8s para iniciar grupos; timer do loop
conserva15s. Isso limita capacidade após progresso; o inspector antigo não diz
quantos dos327 foram adiados por orçamento. Não declarar esse número inteiro
como causa confirmada nem prometer normalização antes do piloto.

## Correção e contratos

HUB habilitado com ACK positivo e lote cheio ou grupos não despachados solicita
uma próxima vez em1s após terminar ambos os flushes. O loop principal volta ao
select, sem goroutine/ticker de envio adicional. Fora desse caso, timer original.
Fila vazia/falha total/cancelamento não gira em loop. Direct/relay preservados.
Janela8s,8threadsmáximas,limite8MiB,auth+identidade+rota e cooldown preservados.
Pruning não é executado mais cedo que a cadência normal; política anterior de
idade/poragente/tamanho não mudou. Não há limpeza/replay/manualACK.

LastDeferred conta envelopes de grupos ainda não iniciados quando o contexto
do dispatcher termina. LastRetained inclui também recusas/falhas; LastReadBatch
registra o lote efetivamente lido. Inteiros seguros entram no snapshot/logs
existentes; sem segredo/payload, nova tabela ou retenção paralela. LastError
continua histórico. HUB_OUTBOX_CATCHUP_ENABLED=false restaura o agendamento
anterior; versão anterior assinada é rollback independente de configuração.

O loop de bootstrap preexistente é grande; novas decisões de cadência ficam em
outbox_flush_cadence.go e o dispatcher separado em funções por responsabilidade.
Integração limitada aos dois flushes existentes, sem refatorar outros coletores.

## Validação local

- Dispatcher: cancelamento controlado com2ACK/14retidos, depois14ACK sem duplicar;
  todos os16IDs confirmados uma vez. RecusaACK nunca vira entrega.
- Policy: HUB/progresso/lote cheio ou deferral; falha total, cancelamento,
  partial pequeno, direct/relay, intervalo já curto e opt-out preservados.
- Credencial: HTTP401permanece pendente; próximo Execute não envia durante60s;
  relógio controlado avança61s e a tentativa volta, sem aceitar o token.
- Pruning: datas fixas comprovam que turnos em1s/9s/14s não antecipam15s.
- Timer real: próxima execução curta sem mudar o intervalo normal de15s.
- Bolt+HTTP loopback:901frames(900válidos/1inválido),615identidades sintéticas,
  lotes800e102. ACK799+101,900IDs recebidos uma vez,auth/rotas isoladas,
  concorrência≤8,401tentado uma vez e mantido após fechar/reabrir a outbox.
  PASS com-race. Fixtures próprias, sem coletas/credenciais produtivas.
- Mutantes controlados de policy/contador foram detectados pelos testes;
  arquivos restaurados e casos focais/race passaram. Evidência de regressão,
  não medição de throughput produtivo.

Check completo `./check.sh` PASS: validações do modelo conforme configuração
legada existente, lint,vet,suiteGo e self-tests de evidência. Race focal e
integração localPASS. CompilaçãoCGO0 Linux/amd64,Windows/amd64 eDarwin/arm64PASS;
binários de teste somente em/tmp, sem pacote/manifesto/assinatura de release.
Review final: boundary/scope,ACK/falhas/cancelamento,cadência/pruning,
contratos aditivos, docs, rollback e ausência deSQL/segredo conferidos.
Nenhum push,artefato oficial,alteração de versão ou update de agente nesta etapa.

Alterações preexistentes staged emDECISOES.md/demandas.md pertencem ao planejamento
PKG-134 e são preservadas. O commit desta correção inclui somente o delta do
incidente, usando índice separado e preservando o delta anterior no índice normal.

## Piloto posterior e riscos

Gate separado com autorização de publicação/update: um HUB, configuração e
outbox preservadas, medir idade collection→received/ACK, e não só tamanho da
fila. VerificarAgentless/relays/coleta de host,CPU/I/O,requisições e latência/API.
A utilização da API pode aumentar enquanto houver progresso e backlog; limites
simultâneos não aumentam. Não prometer recuperação de pontos nunca coletados
ou removidos pela política automática anterior. A credencial inválida requer
origem confiável, sem vincular identidade só por hostname anunciado.
