# PKG-49 e PKG-26 — publicação coordenada

**Implementação e publicação concluídas; homologação/ativação/pilotos separados.**
Usuário autorizou push/publicação do PKG-49 e confirmou explicitamente inclusão
do PKG-26 já implementado na main do Web. Homologações, piloto NOC de30dias,
ativação da prioridade e SLA Agentless permanecem separados.

## Escopo validado

- Web PKG-49:360cf24f; PKG-26:4f82c2c3/c3b8d498.
- Agent PKG-49:9e5debb; fonte da release0.8.66:
  076b0dfbaf8eca52ace8861ce8af8ae0e7fc1ba4, main limpa e sincronizada no GitHub.
- Cinco plataformas compiladas pelo builder oficial, assinatura Ed25519 verificada
  no Go e no catálogo PHP com a chave pública oficial; cinco SHA256 conferidos.
- Build info do binário Linux: fonte exata acima, vcs.modified=false.
- Nova pasta0.8.66 imutável, sem reutilizar a0.8.65 existente no servidor.
  Somente cinco compactados, SHA256SUMS e UPDATE_SIGNATURES.json são distribuídos.
  Nenhuma chave privada é publicada.
- Schema produtivo prévio compatível: auditoria IA com JSON/statusVARCHAR20,
  marcos de observação, projeção do check e catálogo de chamadas existentes.
  Não há SQL de aplicação.

## Esteira e limites

Web publicado por push main/Deploy main, sem pipeline por tag. Diretório de downloads
é preservado pela esteira: os sete artefatos assinados foram publicados em nova pasta atômica pelo
procedimento de distribuição do RUNBOOK, com hash/assinatura e HTTP/Range conferidos. Não usar upload de código ou workflow_dispatch.
Agent não possui pipeline por tag; compilação/assinatura/distribuição são o fluxo
oficial. Nenhum update de HUB/Relay/frota ou mudança de flags é parte desta publicação.
AGENTLESS_DELIVERY_PRIORITY permanece false por padrão; configurações NOC atuais
são preservadas, sem consulta a modelo real nem gasto de IA neste smoke.

Antes/depois conferidos: workflow/SHA/stamp, hashes instalados do código, estado
original de workers, autoload, smoke HTTP e sete hashes públicos dos artefatos.
Homologação/SLA não são inferidos dessa evidência. Recibos locais:
pkg49-local-20261106.md e pkg26-local-20261106.md.


## Evidência instalada — 07/11/2026 UTC (06/11 local)

- Workflow oficial: https://github.com/mayder/aiceberg_web/actions/runs/42322316688
  **success**, commit f0ce186ed9466e2865063bb99a17f9a9b4ac0760.
- Stamp:20261107T010850Z_main_f0ce186ed946; finished_at_utc2026-11-07T01:09:01Z;
  SHA instalado exatamente igual ao commit da pipeline.
- SHA256 dos35arquivos de aplicação/manuais dos PKG-26/49:35correspondências,
  zero divergências. Autoload oficial e php yii help aprovados; cinco classes
  novas NOC/Agentless carregadas diretamente no PHP instalado sem chamar modelo.
- Maintenance renovado graciosamente pela esteira e RUNNING; demais workers
  permanecem STOPPED, como no inventário anterior. supervisorctl retorna3 nessa
  combinação esperada; isso não é falha do deploy nem autorização para reativá-los.
- Schema preexistente confirmado por leitura de metadados, sem DDL ou dados novos.
- HTTP:login Cliente200, site200, API sem token401, IA NOC sem sessão302. Sem
  login sintético, invocação de provedor, alteração de check ou fila produtiva.
- Distribuição oficial assinada: staging com hashes; instalação somente dos sete
  arquivos públicos em pasta nova0.8.66/nginx, ativada por rename atômico, sem
  sobrescrever versões. Chave privada permaneceu local. Não houve upload de código
  nem workflow_dispatch; distribuição de binários é o fluxo documentado separado.
- Catálogo PHP instalado verificou cinco assinaturas oficiais. Os sete arquivos
  foram baixados da URL pública e conferidos contra SHA local; cinco archives
  responderam206/Content-Range bytes0–63 com64bytes corretos.

| Arquivo público0.8.66 | SHA256 |
| --- | --- |
| SHA256SUMS | 32b60fa476ac14e6790b866afdd0b2c916ec03cf884572983108fe7de87e1eb1 |
| UPDATE_SIGNATURES.json | 0661175471104704810c68ff28519fc5851f8a773321a575c41c13a0a414539c |
| aiceberg-agent-darwin-amd64.tar.gz | c83642f68ef81893f125542b3b41b6bb93fd40da3acae04964b502d39d4fec7b |
| aiceberg-agent-darwin-arm64.tar.gz | 11798edc7956771f6d1cb88a683ab4e7a6ff785293185e8ba4e166ce521f239e |
| aiceberg-agent-linux-amd64.tar.gz | bb22a241b734e9908054a0d6a4c8cd88e695af27e104dd2558414fbdc022f79a |
| aiceberg-agent-linux-arm64.tar.gz | 9b2e33bc0fef3266b4635269e5611eac9ddd61e940b606b5384241b677b7271b |
| aiceberg-agent-windows-amd64.zip | 17350c20ad117e8943ddb92ef9864a58814b843923591bfbccb4349cc6cf4aaf |

Base pública:https://cliente.aiceberg.com.br/downloads/agent/0.8.66/.
A versão do binário é0.8.66; source commit076b0dfbaf8eca52ace8861ce8af8ae0e7fc1ba4,
confirmado no build info com vcs.modified=false, ligado aos compactados assinados.
O commit posterior deste recibo é documental e seguirá a mesma esteira; conferir
stamp/smoke novamente sem inferir nova implementação ou homologação desse avanço.

## Gates que permanecem abertos

- PKG-49:escolha/autorização de HUB piloto, atualização instalada e confirmação
  de versão, ativação gradual, relógios, backlog/erro/retry/rollback, medição real
  antes/depois e prova dos1–2min. Nenhum HUB/Relay/frota atualizado neste ciclo.
- PKG-26:configuração validada de cliente/modelo canônico, chamada/consumo reais,
  homologação operacional/isolamento/retorno/layout e piloto com baseline/amostra/
  aceite de30dias. Publicação não é ativação nem comprovação de valor.
- Correções de dependências/advisories globais preexistentes não integram estes
  pacotes; esta publicação não declara auditoria de segurança global concluída.

Rollback preferencial:prioridade false no HUB (já é default); IA assistiva pode
ser desabilitada por flag do cliente em operação autorizada. Não apagar backlog,
observações, auditoria ou feedback. Rollback de código por revert/push oficial;
preservar a proteção contra ACK200parcial. Backup incremental da esteira em
runtime/deploy_backups/20261107T010850Z_main_f0ce186ed946/rsync/.
