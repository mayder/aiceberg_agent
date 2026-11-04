# Candidato 0.8.61 — HUB SADA — 04/11/2026

## Escopo e baseline

Candidato local, sem push/publicação/agendamento. Preparar os cinco pacotes
assinados pelo procedimento do RUNBOOK; não sobrescrever versões anteriores.
A versão0.8.60 já existe no catálogo local e em produção;0.8.61 não existe
no catálogo produtivo verificado em04/11 às08:13SP.

O Linux/amd64 0.8.60 local informa revisão4e595b3544e8f65796751c05836a0c7386619400,
modified=false. Após essa revisão, main contém a preparação administrativa
Windows3429ba5/teste28df66a e as correções HUB610fead/c5c8904. A ferramenta
Windows não é copiada por build_installers.sh e não é executada pelo agente.
Os scripts Linux não mudaram desde a baseline0.8.60. O HUB84 ainda reporta0.8.57:
seu salto inclui também as correções de readiness/updater/política remota
anteriores a 0.8.60, documentadas no RUNBOOK. Não apresentar o salto como apenas
uma alteração de timer. Não ampliar rollout para outros agentes.

Código funcional: envio após persistência em worker único, catch-up após ACK,
limites e cooldown preservados, sem purge/replay/SQL. Evidências detalhadas:
`hub-outbox-arrival-worker-20261104.md` e `hub-outbox-catchup-20261103.md`.

## Estado produtivo antes do piloto

Em04/11 às08:12SP, HUB84/cliente2 reporta0.8.57. Snapshot81034815:
8057itens/190270972bytes, coleta→recebimento280,1s. As seis amostras mais
recentes variam entre278,1 e288s; não é recuperação nem SLA cumprido.
Servidor Web em a411da24, nginx/php-fpm/redis ativos; disco raiz81%/17GiB livres,
auxiliar56%/6,8GiB livres. Inspeção somente leitura, sem nova observabilidade
remota ou reinício. Tamanhos medidos: uploads16GiB, ingest_errors6,5GiB,
logs API2GiB, /var/log3,1GiB, Redis primário4,6GiB (réplica em outro volume). Esses dados não são classificados
como lixo; nenhum apagamento foi feito nesta retomada.

## Gates do piloto autorizado posteriormente

1. Verificar cinco hashes e assinaturas Ed25519; registrar SHA/versão dos
   binários e diferença frente à baseline. Não distribuir chave privada.
2. Publicar somente após autorização explícita de push/publicação. Agendar
   atualização somente do HUB84 no fluxo individual assinado existente.
3. Preservar binário/config/prefs/outbox; confirmar Bolt ativo e espaço local
   antes de depender da persistência após restart. Não reconfigurar armazenamento.
4. Confirmar versão0.8.61, serviço, heartbeat, relays e Agentless; medir
   timestamps reais de coleta e recebimento, ACK/deferred/retenção e recursos
   durante a drenagem. Queda do tamanho da fila sozinha não prova entrega:
   pruning preexistente10min/100por agente permanece e pode descartar dados.
5. Caso haja regressão de serviço/coleta ou crescimento sustentado de atraso,
   reverter somente HUB84 para pacote anterior assinado, preservando estado.
   ASYNC=false restaura envio inline; CATCHUP=false restaura cadência periódica,
   mediante procedimento autorizado. Não afirmar flags remotas sem suporte real.

Instalação real, carga produtiva e recuperação de atraso continuam pendentes.
`./check.sh` passou em04/11 durante a preparação da versão; o hook normal
revalida antes do commit. Arquivos staged preexistentes do PKG-134 em
DECISOES.md/demandas.md devem permanecer fora deste commit.
