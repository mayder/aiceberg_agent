# Definição de itens — validação local20/11/2026

Incremento do PKG-115, sem fechamento de L08/pacote, integração ativa ou release.
Na main existente, sem branch/worktree novos. Domínio monitoring ganha definição
versionada por cliente/ativo/check, tipos/unidades, frescor/retenção e dependências
limitadas. Toda validação exige autoridade resolvida fora do payload. Campo livre,
segredo, chave divergente, mestre ausente/desabilitado, ciclo e excesso são erros.
O codec Go limita256KiB/documento único/profundidade e rejeita campos extras ou
duplicados. Não implementa cadastro, banco, runtime de templates/macros/pipelines,
processamento de séries/eventos, HA ou execução de coletores universais.

DescribeSNMPItems recebe o job existente e política explícita para descrever as
duas séries numéricas da projeção anterior. Não usa intervalo/config do job como
política; não copia endpoint, SNMP profile/community ou dados arbitrários. Não
coleta, grava ou envia. Não promove probes diagnósticos a itens de rotina.
Configuração/assinatura/capability/autoridade operacional continuam no caller;
definição válida não comprova coleta CPU/memória ou dado real do equipamento.

## Testes executados

-52vetores canônicos idênticos em PHP/Go: tipos, escopo/autoridade, identidade,
limites, DAG, mestre desabilitado,128itens, profundidade32 e casos recusados.
-PHP:5testes/206assertions incluindo projeção anterior e schema versus semântica.
-Runtime e Agent, antes do isolamento: domínio com race e PHP real;52comparações
por consumidor e vetores antigos preservados. Projeção/adapters legados intactos.
-Agent, após adapter: domínio+agentless com race; segredo/endereço ausentes,
mesma identidade dos samples, job preservado, política autoritativa e recusa de
contexto/diagnóstico incompatíveis. Nenhuma leitura física ou coleta adicional.
-Fuzz curto com2workers,5s configurados,597860execuções, sem falha. Cobertura
medida: regras semânticas novas100%, codec acima80%; não é cobertura global.
-Microbenchmark de compatibilidade: cinco rodadas de20000projeções SNMP já
existentes;367,1–616,9ns/op,320B/7allocs por operação em MacM4Pro/arm64.
Mede duas séries puras, sem IO; não prova performance em SADA nem baseline global.

Comandos:
`go test -race -count=1 ./internal/domain/monitoring ./internal/platform/agentless`
`AICEBERG_MONITORING_WEB_ROOT=<Web> go test -race -count=1 ./internal/domain/monitoring`
`go test ./internal/domain/monitoring -run '^$' -fuzz '^FuzzDefinitionWire$' -fuzztime=5s -parallel=2`
Web: scripts/check-monitoring-contract.sh; Runtime: domínio internal/monitoring.
Não foi executado check completo/fechamento; validação focada proporcional ao lote.

## Separação da recuperação de logs

A outra conversa está alterando Web/Runtime, inclusive publicação de análises e
readiness. Após teste local, foram preservados patches completos das mudanças
novas desta frente, com manifests SHA256, fora dos dois checkouts compartilhados.
Somente arquivos/hunks próprios foram retirados; alterações/índice/commits de logs
foram preservados. git apply --check passou para os dois patches na base observada.
Nenhum commit/push desta frente, deploy, SQL, mutação de configuração privada,
reset de fila, chamada de IA ou reinício de serviço foi feito.

Evidência durável no Mac:
/Users/brenomayder/.codex/visualizations/2026/11/19/01a1f164-7312-7f90-b41a-ac59feab842a/monitoring-definitions-20261120/
Arquivos: web-definition-v1.patch, runtime-definition-v1.patch, manifests,
logs dos testes/race/fuzz e baseline. Não é outro checkout ou worktree.
Os contratos Web estarão no caminho docs/contracts/monitoring-definitions-v1.md
após integração. Enquanto só em patch, interop PHP depende de aplicá-lo; SKIP
sem env é explícito e não deve ser apresentado como paridade executada de novo.

O piloto SNMP antigo preservou bytes/modo, mas não copiou explicitamente ACL.
Fontes históricas ficam preservadas e marcadas como não reutilizáveis no recibo
RETIRED-PILOT-SCRIPTS.md; novo piloto exige procedimento ACL-safe testado em Linux.
Nenhum teste desta entrega substitui params-local.php ou depende do watcher.

## Pendências e rollback

Integrar patches somente após encerrar a publicação/recuperação concorrente,
revalidar inventários/PRD e bases, executar interop completo e checks proporcionais.
Amostras/eventos universais, templates/macros/pipelines, consumo/transporte ativo,
persistência, compatibilidade instalada, baseline/aceite permanecem abertos.
Não promover todos os pacotes a100% por validar este núcleo. Rollback local remove
somente arquivos/hunks desta entrega, preservando trabalho concorrente e evidência.
Nenhum dado operacional foi criado/modificado para precisar de rollback.

## Continuação — integração após recuperação de logs

Os patches Web/Runtime foram conferidos por SHA256 e reintegrados na main atual,
preservando os commits locais da outra conversa. O motivo para isolamento foi
encerrado; as seções anteriores são o recibo histórico dessa etapa. Paridade PHP/
Go real e race repetidos nos três repositórios. Regressões schedulerexec,
loganalysis/phpcontrol e três suítes de publicação Web passaram. O Runtime ganhou
composição/preview tipado e comando offline, sem duplicar o motor no Agent.
Persistência/versionamento de fontes, APIs/UI e ativação continuam pendentes;
o editor legado que apaga/recria itens não pode servir de base de rollback ativo.
Nenhum deploy, mutação privada, provider ou SQL foi acionado nesta continuação.

Integração coordenada solicitada pelo titular em20/11: fonte do Agent permanece
inativa; composição e prévia privada ficam no Runtime, com catálogo versionado
Web. Commit/push deste incremento preservam a fundação testada e não fecham o
pacote nem autorizam promover um contrato ainda sem emissão/consumo ativo. Não
se cria candidata com o mesmo número de versão, nem se atualiza sensor/agente
por código de domínio que ainda não entrou no caminho de execução.
Check completo ./check.sh aprovado antes do commit; PHP/Go compartilhado já
qualificado. Publicação de binário/aceite instalado continuam gates separados.
