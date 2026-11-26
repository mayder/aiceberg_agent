# Confiabilidade de entrega — 25/11/2026

## Código, publicação e piloto

- 7a751e41: contabilidade Bolt transacional; versão 0.8.72-rc.3.
- 5678aa94: agrupamento opcional somente HUB, validando assinaturas individuais; 0.8.72-rc.4.
- 735e6b83: compatibilidade com campos opcionais do contrato PHP e contagens transitórias redigidas; 0.8.72-rc.5.

Todos passaram hook normal ./check.sh integral, testes race pertinentes e oráculo PHP real. Branch main mantida, sem worktree nova, código SNMP preexistente preservado.

Procedimento oficial do Agent é publicação manual de pacotes assinados (não há pipeline de tag neste repositório). Cinco plataformas por candidata, SHA256SUMS e UPDATE_SIGNATURES.json; cinco assinaturas Ed25519 e cinco checksums verificados antes de publicar. Binário Linux rc.5 com vcs.revision=735e6b834a600e925fad9ae0e979af50178f1a70 e vcs.modified=false.

Arquivos publicados em diretórios imutáveis de versão, HTTP200. SHA256 Linux rc.5: 240911a441dcf3cb963207a8cd933672487001990925bc7b23fb972dfa57a166. Somente HUB84 atualizado pelo atualizador oficial, com readiness fresco aprovado, backup privado e CAS de configuração 74→75→76→77. rc.5 version_confirmed/config_version_applied77 às01:31:42UTC de26/11, PID3594401. Nenhuma atualização de relay.

## Resultado e limites

Antes: fila aproximadamente12mil itens/200MiB, rejeições por lotação, ACK parcial e deferred. Totais da outbox eram calculados por varredura integral a cada gravação/consulta. Corrigidos com totais na mesma transação, reconstruídos na abertura para compatibilidade com rollback. Benchmark local de Len:103644→239ns,4416→10alocações; não representa SLA instalado.

A contabilidade isolada não comprovou recuperação da vazão. O agrupamento assinado foi validado em fixtures, mas nos lotes reais observados os800 cabeçalhos mantiveram fallback conservador (invalid_claim) e grupos de entrada/despacho permaneceram iguais. Isso é resultado do verificador local, não motivo de rejeição da API. Não afirmar que a hipótese de nonce foi confirmada na frota.

À01:35UTC:12546itens/209664269bytes,idade607s,577deferred/800,HTTP401 em logs/raw,ACKs válidos continuando. Sem escoamento sustentado comprovado. API registra invalid_token/inactive_agent e ingest_scope_denied, mas o agente58 com negação de escopo pertence ao HUB10: não atribuir esse registro ao HUB84. Agente625 está inativo e não foi reativado. Correção de credenciais exige vincular o principal real, não substituir autorização pelo hostname nem apagar fila.

Windows585/AWS-CTX-DTS-24 continua sem canal/amostra nova; fonte14:36:25BRT e recepção14:41:37BRT. Titular informou ausência de acesso e deixou verificação local para amanhã. Alteração no HUB não comprova recuperação desse Windows.

Recursos: referência antes do piloto, delta79.937s do mesmoPID2450789:87.69% de um núcleo,RSS3766.2MiB. Amostras de rc.3/rc.4 ficaram perto de900MiB; as recebidas após atualização podem pertencer ao PID antigo por atraso da fila. Comparar apenas timestamps de origem e deltas do mesmoPID. Não usar queda após reinício ou lifetime CPU como prova de ganho sustentado.

## Segurança e rollback

Sem remoção manual, replay ou mudança da retenção/ACK. Falhas não recebem confirmação antecipada. Campos/claims inválidos conservam os limites antigos. Concurrency8, batch800 e capacidade200MiB preservados. Sem mudança em API/Runtime, current, baseline, pin ou DLQ; sem SQL de schema.

Backups privados por etapa em /var/lib/aiceberg-runtime-private/telemetry-delivery-20261125, incluindo hub84-config-before-rc5.json com configuração anterior e pacote de rollback assinado. Rollback pelo atualizador oficial deve preservar prefs/outbox e usar configuração individual guardada, nunca restaurar cegamente a versão global. Pacotes anteriores publicados mantidos.

Entrega permanece parcial: aviso Web instalado/validado em detalhe585 e NOC70997 (1cee29c3), custo da fila corrigido, mas autenticação e vazão ainda exigem fechamento. Não encerrar itens de recuperação no backlog sem novo recibo natural do mesmo servidor/volume e idade normalizada.
