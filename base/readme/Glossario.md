# Dicionário do projeto

Segue um **dicionário/glossário** dos termos, sistemas e tecnologias para o agente NOC+SOC. Definições **curtas e práticas**.

## Conceitos gerais

* **Agente**: programa que roda na máquina (Linux/Windows) coletando dados e executando ações.
* **NOC**: Network Operations Center; foco em disponibilidade, performance e SLAs.
* **SOC**: Security Operations Center; foco em segurança, ameaças e resposta a incidentes.
* **Módulo**: parte do agente que implementa uma função (ex.: sysmetrics, journald, winlog).
* **Plugin**: extensão do agente (processo separado) que fala via contrato gRPC.
* **Política (Policy)**: configuração central que liga/desliga módulos, define limites e regras.
* **Inventário**: dados sobre hardware, software, patches e usuários da máquina.
* **Telemetria**: métricas, eventos e logs enviados pelo agente.

## Coleta (sistemas/tecnologias)

* **CPU/Memória/Disco/Rede**: métricas de uso de recursos do sistema.
* **journald**: serviço de logs do systemd no Linux.
* **Windows Event Log**: infraestrutura de logs do Windows (canais Security/System/Application).
* **ETW (Event Tracing for Windows)**: mecanismo de rastreamento de eventos de alta performance no Windows.
* **WMI (Windows Management Instrumentation)**: interface para inventário e consultas no Windows.
* **SNMP**: protocolo para coletar informações de dispositivos de rede.
* **Antivírus/EDR**: soluções de proteção de endpoint (ex.: Microsoft Defender).
* **Firewall local**: regras de rede da máquina (iptables/nftables no Linux; Windows Firewall).

## Observabilidade

* **Prometheus**: sistema de métricas “pull”; coleta de `/metrics`.
* **/metrics**: endpoint HTTP que expõe métricas no formato Prometheus.
* **/health**: endpoint HTTP de verificação de saúde do agente.
* **Logs estruturados (JSON)**: logs em formato JSON para fácil ingestão e análise.
* **Grafana**: painel visual (dashboards) para métricas do Prometheus.
* **Loki**: sistema de logs da Grafana Labs (opcional para centralizar logs).

## Comunicação & protocolos

* **gRPC**: framework de RPC de alta performance sobre HTTP/2.
* **mTLS**: mutual TLS; cliente e servidor se autenticam com certificados.
* **TLS**: criptografia de transporte (HTTPS).
* **OTLP**: OpenTelemetry Protocol (telemetria padronizada).
* **CEF/LEEF**: formatos comuns de eventos para SIEMs.
* **Syslog**: protocolo tradicional de envio de logs (UDP/TCP/TLS).

## Criptografia & identidade

* **CA (Certificate Authority)**: autoridade certificadora que assina certificados.
* **PKI**: infraestrutura de chaves públicas (gestão de CAs/certificados).
* **ed25519**: algoritmo de assinatura digital rápido e seguro.
* **Assinatura de binário/política**: garante integridade/autenticidade de arquivos e configs.
* **Rotação de certificados**: substituição periódica de certificados antes do vencimento.

## Armazenamento local & filas

* **bbolt (BoltDB)**: banco KV embutido (arquivo único) usado como fila/WAL.
* **WAL (Write-Ahead Log)**: técnica de escrita que garante durabilidade mesmo com falhas.
* **Backoff exponencial**: esperar intervalos crescentes entre tentativas de envio.
* **TTL (Time To Live)**: tempo de vida de itens na fila antes de expirar.
* **Watermark de disco**: limite de uso de disco que dispara limpeza/controla ingestão.

## Execução/Runtime

* **systemd**: gerenciador de serviços no Linux (unidade `.service`).
* **Windows Service**: serviço do Windows gerenciado pelo Service Control Manager.
* **SIGTERM/SIGINT**: sinais de encerramento no Unix; usados para shutdown gracioso.
* **Hot reload**: recarregar config/certificados sem derrubar o processo.
* **Least privilege**: executar com o mínimo de permissões necessárias.

## Empacotamento/Distribuição

* **.deb/.rpm**: formatos de pacote para distribuições Linux (Debian/Ubuntu e RHEL/Alma).
* **MSI (WiX/Advanced Installer)**: instalador para Windows.
* **Winget/Chocolatey**: gerenciadores de pacotes no Windows (opcionais).
* **StateDirectory**: diretório de dados administrado pelo systemd.

## Linguagem & libs (Go)

* **Go (Golang)**: linguagem compilada, binário único; ótima para agentes.
* **gopsutil**: biblioteca Go para coletar métricas de sistema (CPU/memória/disco/rede).
* **zap**: logger de alta performance para Go (logs estruturados).
* **client_golang**: biblioteca oficial do Prometheus para Go.
* **go-systemd/sdjournal**: bindings Go para ler o journald.
* **golang.org/x/sys/windows/svc**: APIs Go para criar/gerenciar serviços do Windows.

## Segurança do SO

* **AppArmor/SELinux**: mecanismos de controle de acesso mandatórios no Linux (hardening).
* **NoNewPrivileges**: opção do systemd para evitar elevação de privilégios.
* **Sandboxing de plugins**: isolar plugins em processos/usuários separados e limitar recursos.

## Regras & detecções (SOC)

* **Sigma**: formato “agnóstico” de regras de detecção (traduzível para consultas específicas).
* **YARA**: linguagem de regras para identificação de arquivos/memória por padrões.
* **IOC**: Indicadores de Comprometimento (hashes, domínios, IPs, artefatos).
* **Brute force**: múltiplas tentativas de login falhas (SSH/RDP etc.).
* **Lateral Movement**: técnicas para se mover entre máquinas dentro da rede.

## Padrões de release & atualização

* **Blue/Green**: manter duas versões e alternar o tráfego entre elas.
* **Canary**: liberar para um subconjunto pequeno antes do rollout total.
* **Rollback**: retornar à versão anterior se a nova falhar no healthcheck.

## Integrações & ecossistema

* **SIEM**: plataformas de eventos de segurança (Splunk, QRadar, Sentinel, Wazuh etc.).
* **EDR/XDR**: soluções de detecção e resposta em endpoints.
* **CMDB/ITAM**: bancos de dados de ativos (inventário) e gestão de TI.
* **Kubernetes/Docker**: orquestração/containers (métricas e eventos de nós/containers).
* **NGINX/Apache/DBs**: serviços comuns com checagens de saúde e métricas específicas.

## Termos de performance/qualidade

* **SLA/SLO/SLI**: acordo/objetivo/indicador de nível de serviço.
* **P95/P99**: percentis de latência (ex.: 95% das requisições abaixo de X ms).
* **Overhead**: consumo adicional de recursos causado pelo agente.
* **Backpressure**: controle de fluxo para não sobrecarregar filas ou rede.

## Formatos de dados

* **JSON**: formato principal de envelopes/events/métricas de debug.
* **Protobuf**: serialização binária usada pelo gRPC (leve e rápida).
* **YAML**: formato legível para configurações locais do agente.

## Rede & checagens ativas (NOC)

* **Ping/ICMP**: teste de reachability.
* **TCP/HTTP(s) check**: abrir conexão/consultar endpoint de saúde.
* **DNS check**: resolver nomes e medir latência/consistência.

## Conceitos de segurança operacional

* **Allowlist**: lista de ações/comandos permitidos.
* **Auditoria**: registro de quem fez o quê, quando e por quê.
* **Mascaramento de PII**: ocultar dados pessoais sensíveis em logs/eventos.
* **Zero Trust**: verificação contínua de identidade/estado antes de conceder acesso.

---

[⬅️ Voltar ao README](../../README.md)
