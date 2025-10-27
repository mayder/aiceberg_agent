# Opções de Arquitetura

Abaixo estão **duas arquiteturas completas** para o AIceberg Agent falar com o backend:

1. **HTTP + JSON batelado** (simples e direto)
2. **gRPC + Protobuf com streams** (robusto para grandes volumes e controle em tempo real)

Incluo: fluxo de **envio** (telemetria), **recebimento** (comandos/políticas), **mudanças no backend**, e por fim uma **comparação pró/contra** + **critério de decisão** e **rota de migração**.

---

## 1) HTTP + JSON batelado (simples e direto)

### 1.1 Envio de dados (Agente → Backend)

* **Endpoint:** `POST /v1/ingest`
* **Formato:** JSON **batelado** (array de envelopes).
* **Compressão:** `gzip` ou `zstd` (fortemente recomendado).
* **Autenticação:** mTLS **ou** Bearer token por agente.
* **Idempotência:** header `X-Idempotency-Key` (p/ o lote) + `envelope_id` por item.
* **Backoff:** exponencial com jitter para `429/5xx`.
* **Fila local:** bbolt; TTL; watermark de disco.

**Exemplo (request body):**

```json
[
  {
    "envelope_id": "01JV...A", "tenant_id":"acme", "agent_id":"host-01",
    "schema_version": 1, "kind":"metric", "sub":"sys",
    "ts_unix_ms": 1730123456789,
    "meta":{"os":"linux","host":"web-01"},
    "body":{"cpu_pct":7.1,"mem_pct":43.3,"disk_pct":48.0}
  },
  {
    "envelope_id": "01JV...B", "kind":"event","sub":"journald",
    "ts_unix_ms": 1730123456799,
    "body":{"unit":"sshd.service","msg":"Failed password for root from 203.0.113.9"}
  }
]
```

**Resposta (ACK por item):**

```json
{"received": 200, "stored": 198, "failed":[{"envelope_id":"01JV...AA","error":"duplicate"}]}
```

#### Backend (o que implementar)

* **API Ingest** (REST): valida auth, valida schema, grava em fila/DB.
* **Dedupe** por `envelope_id` (tabela/estrutura de “seen” com TTL).
* **Persistência:**

  * métricas (timeseries) e eventos (colunar/JSON).
* **Quotas/limites** por agente/tenant → retorna `429` quando exceder.
* **Observabilidade do ingest**: contadores, latências, erros por causa.

---

### 1.2 Recebimento de comandos (Backend → Agente)

Opção A (mais simples): **Long-Polling HTTP**

* **Agente → Backend:**
  `GET /v1/agents/{agentId}/commands?since=<cursor>&timeout=30s`
  O servidor **segura** a conexão até 30s. Se houver comando, responde; senão, retorna vazio. Agente reabre.
* **Resposta:**

```json
{"cursor":"abc-124",
 "commands":[
   {"cmd_id":"c-991","type":"set_policy","payload":{"modules.soc.detections.ruleset":"strict"}},
   {"cmd_id":"c-992","type":"run_action","payload":{"action":"restart_service","name":"sshd"}}
 ]}
```

* **ACK:** `POST /v1/agents/{agentId}/acks` com `[{cmd_id,status,error?}]`.

Opção B (quando quiser menor latência): **WebSocket**

* Canal único WS `GET /v1/agents/{id}/ws` autenticado (mTLS/Token).
* Mensagens JSON nos dois sentidos (telemetria opcional + comandos).
* Mantém semântica de `cmd_id`, `ACK`, `cursor` para reentrega se cair.

#### - Backend (o que implementar)

* **Fila de comandos por agente** (ex.: Redis Streams/DB).
* **Endpoint Long-Poll** (ou WS) com cursor; **expiração** de comandos antigos.
* **ACK**: marcação de entregue/executado; auditoria (quem enviou/resultado).
* **Allowlist** de tipos de comando (segurança).

---

## 2) gRPC + Protobuf com streams (para grandes volumes)

### 2.1 Envio de dados (Agente → Backend)

* **Serviço:** `AgentTelemetry.SendTelemetry(stream Envelope) returns (AckSummary)`
* **Transporte:** HTTP/2, **streaming** de mensagens binárias (Protobuf).
* **Compressão:** builtin (gzip) + compactação por frame; menor overhead que JSON.
* **Backpressure:** nativo (controle de fluxo do HTTP/2 + app-level).
* **Idempotência:** `envelope_id` verificado no servidor; resumo em `AckSummary`.

**.proto (essência):**

```proto
message Envelope {
  string envelope_id = 1;
  string tenant_id   = 2;
  string agent_id    = 3;
  string kind        = 4;  // metric|event|detection|heartbeat
  string sub         = 5;
  int64  ts_unix_ms  = 6;
  bytes  body        = 7;  // msg específica (pb) ou JSON compactado
  map<string,string> meta = 8;
  uint32 schema_version = 9;
}
service AgentTelemetry {
  rpc SendTelemetry (stream Envelope) returns (AckSummary);
}
message AckSummary {
  uint64 received = 1;
  uint64 stored   = 2;
  repeated string duplicates = 3;
}
```

* **Servidor gRPC** (termina stream, valida, grava).
* **Dedupe** por `envelope_id`.
* **Persistência** separando métricas/eventos como no HTTP.
* **Cotas** e **janela deslizante** por agente (encerra stream ou envia sinal de “slow down”).
* **Observabilidade** per-stream: bytes, QPS, erro, reaberturas.

---

### 2.2 Recebimento de comandos (Backend → Agente)

* **Serviço:** `AgentControl.ReceiveCommands(AgentHello) returns (stream Command)`

  * Agente **abre** o stream e fica ouvindo comandos (push).
  * Para enviar resultados/ACKs, duas opções:
    a) método separado `POST Acks` (HTTP/gRPC simples), **ou**
    b) outro serviço `AgentAck.Ack(EventAck)` (unário gRPC).

**.proto (essência):**

```proto
message AgentHello { string agent_id = 1; string version = 2; repeated string capabilities = 3; }
message Command   { string cmd_id = 1; string type = 2; bytes payload = 3; }
service AgentControl {
  rpc ReceiveCommands (AgentHello) returns (stream Command);
}
```

* **Gerador/roteador de comandos** por agente.
* **Stream manager** (lida com reconexão, cursor lógico para reenvio).
* **ACK API** (gRPC unário) e auditoria.
* **Allowlist** e **policy** de permissões.

---

## 3) Comparação: pró, contra, quando usar

| Critério                   | **HTTP + JSON**                                   | **gRPC + Protobuf**                                     |
| -------------------------- | ------------------------------------------------- | ------------------------------------------------------- |
| Implementação inicial      | **Muito simples**; fácil debugar com curl/Postman | **Mais complexa**; requer .proto, codegen e server gRPC |
| Tamanho/CPU por evento     | Maior (texto)                                     | **Menor** (binário); **melhor** para alto volume        |
| Batching                   | Fácil (array JSON + gzip)                         | Nativo (frames); ainda pode agregar no app              |
| Latência                   | Boa; **melhor** com keep-alive/HTTP/2             | **Excelente**; streaming bidi e controle de fluxo       |
| Canal de controle          | **Long-poll/WS** adicional                        | **Nativo** via stream (ReceiveCommands)                 |
| Idempotência               | Simples (headers + IDs)                           | Simples (IDs + AckSummary)                              |
| Observabilidade de fio     | Fácil (logs HTTP, proxies)                        | Menos trivial; exige ferramentas gRPC                   |
| Evolutividade de schema    | Fácil (JSON tolera campos novos)                  | Ótima (Protobuf com campos opcionais e versões)         |
| Interop linguagens         | Universal                                         | Muito boa (SDKs em várias linguagens)                   |
| Esforço backend            | **Baixo a médio** (REST + filas)                  | **Médio a alto** (infra gRPC + streams)                 |
| Escala > bilhões/dia       | Pode exigir mais CPU/custos                       | **Mais eficiente**; melhor custo/benefício              |
| Firewall/proxy corporativo | HTTP padrão facilita                              | gRPC/HTTP-2 pode exigir ajustes/proxy compatível        |

### **Resumo prático**

* Se você quer **colocar de pé rápido** e com **menor esforço**: **HTTP + JSON** (com batching + gzip + long-poll).
* Se você mira **grande escala**, **latência baixa**, **controle em tempo real**, e **eficiência de custo**: **gRPC + Protobuf**.

---

## 4) Esforço e complexidade (qualitativo)

* **HTTP + JSON**

  * Agente: transporte simples (lote + retry + long-poll), pouca infra nova.
  * Backend: 2 rotas REST (ingest/acks) + 1 rota GET long-poll (ou WS) + armazenamento.

* **gRPC + Protobuf**

  * Agente: camada de streams, reconexão, codegen dos stubs.
  * Backend: servidores gRPC (dois serviços), controle de stream, observabilidade específica.

---

## 5) Critérios de decisão

Escolha **HTTP + JSON** se:

* você quer **MVP rápido**, baixo atrito em redes/proxies, depuração simples;
* o volume inicial é **moderado** (ex.: < centenas de milhares de eventos/min global);
* você aceita **implementar WS/long-poll** para comandos.

Escolha **gRPC + Protobuf** se:

* precisa de **streaming de controle** imediato e telemetria **quase em tempo real**;
* a volumetria tende a **crescer muito** (eficiência binária importa);
* você quer contratos **tipados** e **versionamento** forte desde cedo.

---

## 6) Rota de migração (começar simples e evoluir)

1. **Comece com HTTP + JSON** (ingest + heartbeat + long-poll).
2. **Padronize o envelope** (os mesmos campos que usaremos no Protobuf).
3. **Abstraia o transporte no agente** (interface `Transport` com implementação `httpjson`).
4. Quando (e se) precisar, **adicione `grpcstream`** mantendo o mesmo envelope.
5. O backend pode rodar **HTTP e gRPC em paralelo** durante a transição.

---

## 7) Desenhos de alto nível

### HTTP + JSON + Long-Poll

```markdown
[ Agent ] --POST /v1/ingest (JSON batch + gzip)--> [ API Ingest ] --persist-->
[ Agent ] <--GET /commands?since (long-poll)-------- [ API Control ]
[ Agent ] --POST /acks------------------------------> [ API Control ]
```

### gRPC + Protobuf (Streams)

```markdown
[ Agent ] ==stream Envelopes==>  [ gRPC Ingest ] --persist-->
[ Agent ] <==stream Commands==   [ gRPC Control ]
[ Agent ] --Ack (unary)--------> [ gRPC Ack ]
```

---

Adaptar o **MVP** do canvas para a opção que escolhermos (HTTP **ou** gRPC) — com:

* **camada de transporte** isolada,
* ingest/heartbeat prontos,
* controle (long-poll ou stream),
* exemplos de **payloads** e **tests** de reconexão/backoff.

---

Curto e direto, Arquitetura:

### Respostas objetivas

* **GO no agente (Linux/Windows):** **Sim** para as duas opções (HTTP+JSON e gRPC). Go compila nativo para ambos e roda como service/systemd/Windows Service.
* **Backend Yii2 Advanced (PHP):**

  * **HTTP+JSON:** **Sim, nativo** (controllers REST, gzip/zstd, auth).
  * **gRPC:** **Não recomendado direto em PHP** (suporte/operar servidor gRPC em PHP é limitado/complexo). **Use um sidecar/gateway** em Go/Node/Java para terminar gRPC e repassar ao Yii2 por HTTP/fila.
* **Offline e retomada automática:** **Sim** em ambas, usando **fila local** (bbolt/SQLite), **idempotência** e **retry/backoff**.
* **Sem acesso externo (hop interno / relay):** **Sim**, dá para configurar um **Agente Hub** (relay) dentro da rede que recebe dos agentes internos e **reenvia** para a nuvem quando tiver saída (store-and-forward).

---

### Como fica cada opção com Yii2

#### Opção 1 — HTTP + JSON (mais simples)

* **Agente (Go)**

  * Coleta → **lotes JSON** (256–512 KB ou 200–500 ms) → `POST /v1/ingest` (gzip/zstd).
  * **Heartbeat** `POST /v1/heartbeat`.
  * **Comandos** via **Long-poll** `GET /v1/agents/{id}/commands?since=cursor` (ou WebSocket depois).
  * **Fila local** + **idempotência** (`envelope_id`) + **backoff** em 429/5xx.
* **Backend (Yii2)**

  * Controller `IngestController` (valida, de-duplica, persiste).
  * Controller `CommandsController` (fila por agente; long-poll; ACK).
  * Suporta **Linux/Windows** sem diferença do lado do PHP.

**Prós para você agora:** tudo dentro do Yii2; deploy simples; debug fácil.
**Contras:** menos eficiente que Protobuf em volumes muito altos; canal de controle via long-poll/WS (uma peça a mais).

---

#### Opção 2 — gRPC + Protobuf (para grande volume/baixa latência)

* **Agente (Go)**

  * **Stream** `SendTelemetry` (Protobuf) + **stream** `ReceiveCommands`.
  * Mesmo esquema de fila/idempotência/backoff.
* **Backend (recomendado com Yii2):**

  * **Sidecar Ingest/Control** (Go/Node/Java) termina gRPC, aplica cotas/ACK e publica para o Yii2 (HTTP interno) ou **fila** (Redis/Kafka/SQL).
  * Yii2 segue como **plataforma/console/admin/API**.

**Prós:** menor overhead, streaming bidi nativo, melhor custo/latência em escala.
**Contras:** exige **um serviço extra** (sidecar) e observabilidade de gRPC.

---

### Offline, retomada e hub (relay) — igual nas duas opções

* **Offline:** agente escreve em **fila local** (bbolt/SQLite).
* **Retomada:** ao voltar a rede, envia **em lotes**, com **idempotência** para não duplicar.
* **Hub/Relay interno:**

  * Vários agentes **POST → Hub** (LAN);
  * Hub **bufferiza** e **envia** para a nuvem quando possível;
  * Pode atuar também como **broker de comandos** (o backend manda para o Hub; ele entrega via LAN).

#### Desenho (HTTP+JSON; análogo para gRPC trocando o fio)

```markdown
[ Agentes LAN ] --POST batch JSON--> [ Agente Hub / Gateway ]
       |                                   |
       |<-- long-poll / cmds --------------|   (opcional)
                                           |
                               (quando há internet)
                                           v
                               [ API AIceberg (Yii2) ]
```

> Com gRPC, o **Hub** termina gRPC dos agentes LAN e fala com a nuvem por gRPC ou por HTTP/JSON (fazendo transcodificação).

---

### Decisão prática (minimizando esforço agora)

* **Quer começar hoje dentro do Yii2 e ter menos peças?**
  Vá de **HTTP+JSON batelado** + long-poll para comandos.
  (Deixe a camada de transporte do agente **abstraída** — amanhã você pluga gRPC sem reescrever tudo.)

* **Já prevê altíssimo volume/latência baixa e aceita um serviço extra?**
  Vá de **gRPC+Protobuf** com um **gateway/sidecar** em Go na borda do Yii2.

---

### Passos rápidos para cada caminho

### **HTTP+JSON**

1. Criar `POST /v1/ingest` e `POST /v1/heartbeat` no Yii2 (gzip/zstd, token/mTLS).
2. Criar `GET /v1/agents/{id}/commands` (long-poll 30 s) + `POST /acks`.
3. No agente: fila bbolt, batch + retry + idempotência, heartbeat, long-poll.
4. (Opcional) **Hub**: mesmo backend do agente, só muda o destino.

### **gRPC (com sidecar)**

1. Definir `.proto` (Envelope/Telemetry/Control).
2. Subir **sidecar** gRPC (Go) para terminar streams e publicar no Yii2/fila.
3. No agente: streams + fila/idempotência.
4. (Opcional) **Hub**: sidecar gRPC com buffer e forward.

---

[⬅️ Voltar ao README](../../README.md)
