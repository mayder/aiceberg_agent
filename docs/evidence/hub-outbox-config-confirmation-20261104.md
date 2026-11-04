# Confirmação de configuração no HUB — candidato0.8.62

0.8.61/f02b790 foi publicada pelo procedimento de pasta versionada assinada e
instalada somente no HUB84 às08:23:05SP. SHA do binário485420dee8de967374e293506746d06452974f796ba1f5fad4676db422514be0.
Coletas retornaram a0–1s, fila passou de~8mil para~60–120itens; relay124 ficou
recente e Agentless1263/1423 continuaram. Queda de fila não prova entrega de
cada ponto antigo: pruning preexistente10min/100por agente foi preservado.
Recibos privados ficam em production-storage-incident-20261103 no bundle oficial.

OFF foi publicado v41, mas config-report permaneceu v40. Causa reproduzida:
ApplyConfigPayloadWithSecurity persiste versão no ingest; GET posterior com
version=41 recebe204 e não envia report. O callback antecede este worker;
mais rapidez de envio favorece esse caminho. Não afirmar aplicação no host
somente pelo botão OFF nem alterar o banco para simular confirmação.

Correção: wrapper próprio do HUB assíncrono detecta mudança persistida de
versão, marca RequestFullSync e reutiliza o próximo ConfigSync. GET omite
version uma vez; autenticação/assinatura/validação/ACK são os existentes.
Não faz HTTP no callback nem muda a cadência. Se config-report falha,
RequestFullSync permanece para próximo ciclo, inclusive nos demais modos que
já usam ConfigSync; o poll condicional volta após sucesso. Política, comandos,
credenciais, armazenamento, limites, retries de dados e retenção não mudam.

Teste HTTP/prefs em arquivo reproduziu reports=0 antes da correção. Teste corrigido
exige ACK aplicado v41 autenticado e depois204 sem outro report. Casos adicionais:
report503 seguido de novo GET completo e sucesso; duplicata/relay/falha de
persistência/versão vazia não pedem sync. Casos do worker/Bolt/relay continuam
no reteste-race. `./check.sh` passou; o hook normal revalida a árvore no commit.

Rollback: pacote0.8.61 assinado no mesmo HUB preservando estado; devolve só a
lacuna aqui corrigida. Sem SQL, restart manual ou nova estrutura de logs.
Candidato0.8.62 é local: não houve push/publicação/update desse candidato.
Piloto autorizado posterior deve registrar config41 aplicada, preferências,
serviço/relays/Agentless e frescura das métricas. Manter OFF ao concluir.
