# HUB — freelist e serialização por rota, candidato0.8.64

## Evidência e limites

0.8.63/917a0be foi publicada e confirmada somente no HUB84 em04/11/2026 às18:24:39 BRT, com SHA do binário validado pelo inspector119. OFF/config50 confirmada às18:33:16. A escrita caiu de32,834MB/s antes do restart para26,667MB/s no recorte inicial (~19%). Nova janela às19:09:49 BRT:27,13MB/s e91 itens; cpu_percent52,21% é média do processo, não leitura instantânea. O arquivo de outbox tinha276.484.096bytes no inspector, mesmo com poucos envelopes. Tamanho alocado não é backlog lógico.

Inspeção do bbolt1.3.7 fixado no go.mod: tx.Commit regrava a freelist em todo commit; NoFreelistSync omite esse índice reconstruível, mantém tx.write/writeMeta e respectivos fsync. NoSync permanece false. Ao abrir arquivo sem freelist persistida, a biblioteca varre páginas alcançáveis para reconstruí-la; o construtor anterior também faz essa recuperação. Abertura pode ficar mais lenta; não representa perda dos dados/identidades. Não mudou biblioteca/formato, não há compactação, truncamento, GC adicional ou retenção nova.

O fluxo de flush também serializava todos os envelopes no normalizador de logs antes de testar sua rota. Assim, métricas — inclusive retidas por cooldown — eram codificadas numa etapa que só divide logs raw. A checagem de rota passa a anteceder essa serialização. splitFlushBatchByJSONSize e o transporte continuam validando a codificação/tamanho das requisições; não remove limite nem aceita corpo inválido.

## Correção e rollback

HUB_OUTBOX_FREELIST_REBUILD_ENABLED=true por padrão, efetivo somente em modo hub, independente do agrupamento de escrita. Flagfalse exige reinício autorizado; pacote0.8.63 reabre o arquivo sem mudar envelopes. Inspector expõe a opção efetiva. Direct/relay continuam persistindo a freelist. O sucesso de Push/ACK continua condicionado ao commit durável; autenticação, retry/cooldown, rotas e retenção não mudam.

## Retestes locais

- Race nos pacotes outbox/usecase/bootstrap/config: commit bloqueado sem sucesso antecipado, falha/oversized concorrente, capacidade/duplicata e ACK, kill após sucesso, reabertura pelo construtor anterior, upgrade/rollback, verificação estrutural bbolt e preservação de identidades/rotas. O encerramento do processo não simula perda física de energia ou defeito do disco.
- Normalização: corpo metrics com MarshalJSON inválido não é acessado pelo normalizador de logs, mas é recusado no envio sem transporte/ACK; raw logs continuam validados. Regressões reais de oversized, HTTP/64relays, timeout/cooldown e limites mantidas.
- BenchmarkBoltFragmentedTrickle: fixture sintética de24mil registros4KiB, escoada para64; chegadas sequenciais de16KiB com Push/Peek/ACK não se agrupam. macOS arm64:311.296→114.688bytes de páginas alocadas/chegada;10→8writes/chegada. CPU/tempo macOS sem melhoria consistente (fsync/custo do host); não alegar ganho de CPU nesse ambiente.
- Mesmo binário de teste em Linuxarm64/Docker, sem rede, filesystemtmpfs próprio:638.976→57.344bytes alocados/chegada (~91%),10→8writes. Duas séries500x:CPU4.905/4.892→4.403/4.429microsegundos/chegada (~10%); tempo15,19/15,30→14,81/14,83ms. Stats.PageAlloc não mede bytes físicos, tmpfs não qualifica durabilidade/capacidade de disco produtivo, e ARM não substitui piloto AMD64.
- BenchmarkMetricLogNormalization,64 métricas sintéticas16KiB: etapa antiga592.680ns/1.197.995bytes/321alocações por passagem; etapa nova250ns/zeroalocações. É apenas o passe redundante removido, não o tempo total do envio ou a CPU integral do HUB.
- Comandos: go test ./internal/data/local/outbox -run '^$' -bench BenchmarkBoltFragmentedTrickle -benchtime=300x -count=2; go test ./internal/domain/usecase -run '^$' -bench BenchmarkMetricLogNormalization -benchtime=1s. Linux: GOOS=linux GOARCH=arm64 go test -c ./internal/data/local/outbox, executar com500x em container próprio sem rede. Fixture exclusivamente sintética, sem dump de cliente.

## Gates

Check completo/review/commit e cinco pacotes assinados são gates locais. Novo push, publicação da pasta imutável e atualização somenteHUB84 requerem autorização própria. Medir deltas do mesmoPID/uptime, escrita, CPU, recursos, ACK/atraso e tempo de recovery; manter0.8.63 para rollback e não alterar dados. CPU integral do HUB continua sem profiler produtivo e não está declarada resolvida. Não fecha PKG-33/134 nem credencial inválida/histórico.

## Consolidação local

./check.sh PASS; reteste final de durabilidade com race PASS. Binário Linuxarm64 executou testes de kill/reabertura/upgrade no ambiente isolado sem rede (helper de subprocesso tem SKIP esperado quando chamado sem caminho; casos pai foram executados e passaram). Os cinco pacotes0.8.64 foram construídos e SHA256/assinaturasEd25519 aiceberg-agent-prod-v1 conferidos em diretório privado novo; sem upload, agendamento, push ou atualização produtiva nesta etapa. Hooks obrigatórios voltam a executar o gate na criação do commit exclusivo. Nenhuma alteração staged do proprietário foi incorporada.
