# HUB — agrupamento de gravações duráveis, candidato 0.8.63

Em 04/11 às 15:29, o HUB84/0.8.62 reportou 1.726 itens, 38.296.837 bytes e 46,1 s entre coleta e recebimento. Dois snapshots do mesmo processo/uptime avançaram 10 s e 393.826.304 bytes escritos (39,4 MB/s). O contador é cumulativo de I/O do processo, não ocupação do disco; cpu_percent do agente é média desde seu início e não CPU instantânea. A causa integral de CPU ainda não foi qualificada por profiler no HUB.

O armazenamento fazia uma transação durável por Push e por ACK de grupo. O mutex externo em Push impedia agrupar gravações concorrentes. A freelist mantida após escoar uma fila grande também é regravada em cada commit. Ensaio controlado de fragmentação confirmou amplificação; não foi copiado estado de cliente.

## Correção local

Somente HUB, HUB_OUTBOX_WRITE_BATCH_ENABLED=true por padrão, usa bbolt.Batch: até 64 callbacks e espera máxima de agrupamento de 5 ms antes de iniciar a transação. Essa espera não limita tempo de commit nem garante latência de entrega. O relay só recebe sucesso depois de commit/fsync. Dados, metadados e freelist continuam sincronizados: NoSync/NoFreelistSync não foram ativados, formato e versão bbolt não mudaram.

Push valida capacidade na própria transação, descontando substituição do mesmo ID; callbacks podem ser repetidos pelo bbolt e não têm efeitos externos. Um item acima do limite falha sem descartar escritas vizinhas válidas. ACK continua explícito e posterior à confirmação remota. Replace, pruning, autenticação, cooldown e envio em segundo plano conservam contratos. Inspector expõe somente a flag efetiva.

## Validação

- Race nos testes de armazenamento/configuração/worker, com HTTP real em loopback e rajada de 64 relays; API bloqueada não bloqueia o recebimento local além da persistência.
- Escrita segurada por transação não retorna ACK antes de commit; oversized concorrente não perde 128 gravações válidas; sobrescrita idempotente não consome capacidade duplicada.
- Processo filho confirma 128 gravações, é encerrado abruptamente sem Close e o arquivo é reaberto pelo construtor anterior: IDs, conteúdo, credenciais sintéticas e rotas preservados. Isso não simula falha física do disco/energia.
- Benchmark do BoltStore real, Mac arm64: 8 rajadas de 32 produtores concorrentes, após escoar fixture sintética de 24 mil registros. Antes: 755,76 ms/rajada, 332,2 operações de escrita e 9.936.896 bytes de páginas alocados/rajada. Depois: 31,72 ms, 25 operações e 557.056 bytes de páginas. Métrica de páginas não é total de bytes físicos do disco; tempo não é CPU medida.
- Comando: go test ./internal/data/local/outbox -run '^$' -bench BenchmarkBoltDurableConcurrentBurst -benchtime=8x -count=1. Retestes focados com -race passaram; ./check.sh obrigatório no hook de consolidação.

## Publicação, piloto e rollback

Candidato local; não instalado nem publicado. Não fecha PKG-33 nem a credencial inválida. Antes de instalar, publicar cinco pacotes assinados conforme RUNBOOK e obter autorização de push/piloto, limitado ao HUB84. Medir deltas de I/O/CPU, atraso real, fila/ACK e recebimento; não aceitar só queda de queue_items como prova de entrega.

Rollback: pacote assinado 0.8.62 com outbox/configuração preservadas; o formato não mudou e a reabertura anterior foi testada. Opt-out operacional HUB_OUTBOX_WRITE_BATCH_ENABLED=false exige reinício autorizado no host. Não apagar/compactar arquivo, não mudar retenção e não ampliar a atualização a outros agentes nesta etapa.
