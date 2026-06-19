# PKG-69 - update remoto real via web

Data UTC: 2026-06-19T11:33:27Z

Escopo:
- Agente canario: `id=4`, `VM_AI_PROD_2`, cliente `1`, Linux amd64, modo direct.
- Versao origem: `0.8.8`.
- Versao alvo: `0.8.9`.
- Artefato oficial: `cliente/web/downloads/agent/0.8.9/aiceberg-agent-linux-amd64.tar.gz`.
- SHA256 do pacote: `7ddaa0bcbbb94c2aaa2994c4170aa669ab6cd3ff854a621ad6a9b2516d828b50`.

Evidencia:
- `0.8.9` foi publicada em `cliente/web/downloads/agent/0.8.9/` somente com compactados oficiais e `SHA256SUMS`.
- A tela `cliente.aiceberg.com.br/agente/view?id=4` abriu autenticada, mostrou `0.8.8 · linux / amd64`, e o menu `Coletar agora > Update remoto` selecionou `v0.8.9 • Linux AMD64`.
- Primeira tentativa pela tela foi consumida pelo agente e falhou de forma controlada porque o canario no proprio VMAIPROD2 nao conseguia conectar em `https://cliente.aiceberg.com.br` via IP publico: `download_failed`, `connect: connection timed out`.
- Auto-update estava inicialmente desabilitado no canario e a primeira tentativa anterior registrou `skipped/feature_disabled`; a policy foi habilitada apenas para o agente `4`, com launcher `/usr/local/sbin/aiceberg-agent-update-launcher.sh`.
- Para concluir o teste funcional no mesmo host, o update foi reenfileirado no contrato web `pending_update_payload` com URL local HTTP resolvivel pelo canario: `http://cliente.aiceberg.com.br/downloads/agent/0.8.9/aiceberg-agent-linux-amd64.tar.gz`. O ajuste temporario de `/etc/hosts` foi revertido apos o teste.
- O apply script registrou:
  - `start update version=0.8.9 file=/var/lib/aiceberg/data/updates/0.8.9/aiceberg-agent-linux-amd64.tar.gz`
  - `restart via systemctl concluído`
  - `update concluído com sucesso version=0.8.9 bin=/usr/local/bin/aiceberg_agent`
- O backend registrou `last_update_status=version_confirmed`, `pending_update_payload=null`, `last_update_target_version=0.8.9`.
- Health local apos update: `status=ok`, `version=0.8.9`, `connected=true`, `flush_ok=6`.
- `systemctl is-active aiceberg-agent=active`, `NRestarts=0`.
- SHA256 do binario instalado: `a25d50df4aec1cbb798d5afcac963b7848a84fcb7c8dcf8a6e7e124f42bfa78d`.
- SHA256 do binario extraido do pacote Linux amd64 local: `a25d50df4aec1cbb798d5afcac963b7848a84fcb7c8dcf8a6e7e124f42bfa78d`.
- Tela do agente apos refresh mostrou `0.8.9 · linux / amd64`, `Config v5` e ping por `channel_heartbeat`.

Limitacao:
- A tentativa estritamente gerada pela UI com URL publica HTTPS expôs pendencia operacional de hairpin/reachability/cadeia TLS no proprio VMAIPROD2 para `cliente.aiceberg.com.br`.
- Em 2026-06-19T12:24:05Z, a pendencia local foi corrigida no VMAIPROD2 com split-horizon local `cliente.aiceberg.com.br -> 127.0.0.1` e inclusao da intermediate `GlobalSign GCC R6 AlphaSSL CA 2025` na trust store do host. A validacao posterior retornou `openssl verify /etc/pki/nginx/server.crt: OK` e `curl -I https://cliente.aiceberg.com.br/downloads/agent/0.8.9/aiceberg-agent-linux-amd64.tar.gz` com `HTTP/2 200`.
- Apos a correcao, o update forçado do agente `4` usando a URL publica HTTPS original confirmou `last_update_status=version_confirmed`, `msg=apply_ok`, health `status=ok`, `version=0.8.9`, `connected=true`.
- A validacao comprova update remoto real, apply, restart e reconexao no canario com URL publica HTTPS funcional no proprio host, mas nao substitui update assinado oficial Ed25519 nem fechamento completo do PKG-69.

Rollback:
- O apply script manteve backup local automatico antes de substituir `/usr/local/bin/aiceberg_agent`.
- Rollback operacional permanece: reenfileirar pacote anterior ou restaurar backup do binario e reiniciar `aiceberg-agent`.
