# 🧠 AIceberg Agent

O **AIceberg Agent** é o componente responsável por coletar, consolidar e enviar dados de telemetria (NOC) e segurança (SOC) para o backend do **AIceberg**, operando de forma autônoma em **Linux e Windows**.  
Esta primeira versão implementa o envio via **HTTP + JSON**, com foco em simplicidade, resiliência e portabilidade.

---

## 📁 Estrutura de Documentação Base

Para manter o repositório mais limpo e organizado, os principais documentos de especificação e requisitos estão localizados na pasta:

```text
/base/readme/
```

Abaixo segue a descrição e o link de cada arquivo.

---

### 🧱 [Estrutura Inicial.md](/base/readme/Estrutura_Inicial.md)

Documento com a **árvore de diretórios**, responsabilidades de cada pasta/arquivo e o **prompt recomendado** para continuar o desenvolvimento (implementação do pipeline de envio HTTP+JSON). Use-o como guia rápido para navegar no projeto e pedir os próximos passos.

---

### 🔷 [Arquitetura.md](/base/readme/Arquitetura.md)

**Status:** em construção  
Contém a **visão de arquitetura completa** do agente, descrevendo camadas, módulos, responsabilidades e o fluxo de comunicação entre agente, backend e hubs intermediários.  
Este arquivo servirá como referência principal para diagramas, decisões técnicas e padrões de implementação (SOLID, ports/adapters, injeção de dependências).

---

### 🧩 [Requisitos NOC.md](/base/readme/Requisitos_NOC.md)

Lista completa e detalhada de **todas as funcionalidades esperadas de um NOC moderno** — incluindo coleta de métricas, monitoramento de rede, análise de disponibilidade, notificações e dashboards de desempenho.  
Serve como guia de escopo para os módulos de telemetria e monitoramento do agente.

---

### 🛡️ [Requisitos SOC.md](/base/readme/Requisitos_SOC.md)

Relaciona as **funcionalidades esperadas de um SOC** (Security Operations Center) — coleta de logs, detecção de ameaças, resposta a incidentes, correlação de eventos e governança de segurança.  
Este documento é usado como base para o roadmap da parte de segurança do agente.

---

### 📘 [Dicionário do projeto.md](/base/readme/Glossario.md)

Dicionário técnico e conceitual com a **definição de todos os termos, tecnologias e siglas** utilizadas no projeto AIceberg Agent.  
Inclui explicações sobre protocolos, padrões, bibliotecas Go e componentes do ecossistema (Prometheus, journald, mTLS, bbolt etc.).

---

### 🚀 [Base pro futuro.md](/base/readme/Base.md)

Documento de visão e continuidade.  
Registra **decisões arquiteturais, planos de expansão, ideias e melhorias futuras** do agente, incluindo a transição para gRPC, novos coletores, e estratégias de atualização e controle remoto.

---

## 🧭 Próximos Passos

1. Implementar o módulo `sysmetrics` (coleta de CPU, RAM, disco, rede).  
2. Criar camada de transporte HTTP (`httpjson`) com compressão e idempotência.  
3. Integrar fila local (`bbolt`) e lógica de reenvio automático.  
4. Validar ingestão no backend do AIceberg.  
5. Atualizar o documento [Arquitetura.md](/base/readme/Arquitetura.md) com os diagramas e decisões tomadas.

---

## ⚙️ Execução Rápida

```bash
make tidy
make run
```

O agente carregará o arquivo de configuração `./configs/config.example.yml`, exibirá logs básicos e iniciará o ciclo de coleta e envio (modo esqueleto).

---

## 📄 Licença

Projeto de propriedade do **AIceberg**, desenvolvido sob orientação do Arquiteto do Caos Elegante.  
Todos os direitos reservados.
