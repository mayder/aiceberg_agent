# CPU do HUB e picos do relay — candidato0.8.65

## Evidência produtiva e limite da atribuição

0.8.64 foi instalada somenteHUB84 às19:34:02 BRT de04/11/2026, commit6d49918, inspector120 e OFF/config53 confirmados. Em42 amostras/410s, escrita26,673→5,933MB/s (-77,76%). CPU exibida52,82→44,87% é média de vida de processos com idades diferentes, não redução causal por janela. Fila59–213/última65 no recorte. Relay124/0.8.52:44 métricas, mediana6,556s/p9529,293s/máximo48,789s até a API. Falta timestamp de ingresso no HUB nos envelopes antigos; não é possível distribuir retrospectivamente esses48,789s entre os saltos. Janela posterior de54 snapshots às19:58:39BRT não tinha novo pico>20s; isso não fecha a intermitência.

No código doRelay, OutboxFlushInterval é15s por padrão e o timer é tratado no mesmo select das coletas/chamadas síncronas. TSUnixMs nasce depois da coleta do próprio envelope; o total pode crescer por espera de envio e outras rotinas subsequentes, transporte/HUB/API ou clocks. A arquitetura permite adiar a transmissão, mas não prova que um coletor específico causou todo o pico histórico.

NoHUB, listener e Bolt.Peek materializavam Body em mapas e o forwarding serializava novamente. Isso se repete em cada leitura de envelopes retidos por cooldown. Reproduzido no store real com50 corpos estruturados de240 amostras cada (~0,94MB total). Não usar simples quantidade de itens como prova de entrega.

## Correção

- HUB_OUTBOX_RAW_BODY_ENABLED=true, somenteHUB: DTO/parser compartilhado valida o JSON e decodifica campos do Envelope, mantendo Body como RawMessage com bytes próprios. Recepção, persistência e forwarding preservam JSON/limites/identidade; Peek não expõe memória da transação. Não muda biblioteca/formato bbolt, fsync, capacidade ou ACK.
- RELAY_OUTBOX_ASYNC_ENABLED=true, somenteRelay: mesmo worker único doHUB acorda por notificação não bloqueante posterior a Append. Coleta não espera envio nem o envio espera o timer/coletor ocupado. Timer continua fallback; lote cheio com progresso retoma em1s. Retenção conserva a cadência normal; HTTP/retry/401 e aplicação de configuração própria no loop principal conservam controles. Direct inalterado.
- meta.hub_received_at_unix_ms é sobrescrito peloHUB após parse/antes deAppend. Mantém timestamp original e permite distinguir antes/depois desta recepção, com ressalva de clock/precisão. A API Web preserva o Envelope no raw_json existente (IngestController::storeSnapshot); sem SQL/tabela/log extra, mesma retenção. Não é credencial, prova de origem ou instante exato do fsync; um retry marca a nova tentativa.
- cpu_time_sec no perfil do processo permite medir CPU por delta do mesmoPID; cpu_percent mantém seu significado anterior. Inspector expõe ambas as flags efetivas.

## Retestes

Race nos pacotes bootstrap/hub/envelopejson/outbox/config/usecase passou. Pipeline real Relay→HTTPHUB→Bolt→HTTPAPI recebe o frame antes de liberar um coletor bloqueado e sem esperar o timer1h do ensaio; Header de auth/identidade e rota preservados. Teste separado503 conserva envelope durável após shutdown/reabertura, semACK. Notificações coalescem e os ciclos não se sobrepõem. Pruning em100 ciclos continua uma vez na cadência, sem replay. Configuração própria continua no loop principal pelos testes existentes.

Parser rejeita JSON inválido e copia RawMessage; alterar o request/registro depois da leitura não altera o Body retornado. Construtor anterior reabre e lê conteúdo/credenciais sintéticas/rota. Listener recusa persistência parcial também no modo opaco. Split de logs usa Bolt real/HTTP com requests≤400B, mantém autenticação e todos os três eventos; o store ordena filhos por ID, portanto o teste não inventa garantia de ordenação global por sequence.

BenchmarkBoltStructuredRetainedRead,100x/duas séries,50 registros: Linuxarm64 isolado sem rede, tmpfs próprio (não qualifica disco produtivo), CPU30.370/32.450→8.237/8.577microsegundos por leitura/forwarding (~73%); tempo17,69/18,93→7,50/7,85ms; alocações349mil→~822 e bytes17MB→3,3MB por passagem. Macarm64:CPU38.969/37.022→7.225/7.223us; tempo15,29/15,63→6,89/6,83ms. São medições da etapa sintética, não da CPU integral doHUB. Não alegar redução produtiva de73% ou SLA antes do pilotoAMD64.

## Gates e piloto

Check completo/review/commit e cinco pacotes assinados são gates locais. Publicação de0.8.65 exige novo push autorizado, pasta imutável oficial e verificação pública dos sete arquivos. Piloto solicitado: primeiroHUB84 (comRelay124 ainda0.8.52) para medir saltos, depois somenteRelay124. Medir CPU por janela, fila/ACK, intervalos dos eventos e p95/máximo em cada trecho; não ignorar picos na seleção. Inspeção temporária deve retornarOFF confirmado. Não ampliar à frota ou declarar49s resolvidos por microbenchmark.

Rollback: flagsfalse após reinício autorizado ou pacotes anteriores assinados e conferidos, HUB0.8.64/Relay0.8.52 conforme plataforma/catálogo. Arquivos de fila preservados, sem limpeza/compactação e sem replay. Credencial inválida e27confirmações históricas continuam separadas; nenhuma alteração de dados realizada. Não fecha PKG-33/134.

## Escopo atual do piloto

Leitura autenticada de04/11 às20:01:15BRT confirmouRelay124/cliente2, OCI-LIC-PRD-01, Windows, modoRelay e hub_agente_id84; métricas trazem via=hub. O piloto desseRelay requer aprovação própria, distinta da atualização anterior somenteHUB84. Não é a origem não autenticada da credencial inválida.

Na ativação do workerRelay, a primeira varredura de retenção espera a cadência normal; chegada/startup não antecipa pruning. Essa proteção não desliga a retenção já existente. Não há operação manual de limpeza ou histórico. Check completo passou; gate do hook e artefatos assinados são registrados no recibo do candidato antes de pedir publicação.
