# **Requisitos SOC:**

---

## 🔍 **Coleta e Ingestão de Dados**

* Coleta de logs de segurança do sistema operacional (Windows Event Log, journald, syslog).
* Coleta de logs de autenticação (SSH, RDP, Kerberos, LDAP, AD, sudo).
* Coleta de logs de rede (firewalls, switches, IDS/IPS, VPN, proxy, DNS).
* Coleta de logs de aplicativos (servidores web, e-mail, banco de dados, ERP).
* Coleta de eventos de antivírus/EDR (quarentenas, detecções, atualizações).
* Coleta de logs de cloud (AWS CloudTrail, Azure Activity Logs, GCP Audit Logs).
* Coleta de logs de containers e orquestradores (Docker, Kubernetes Audit Logs).
* Coleta de eventos de autenticação SaaS (Microsoft 365, Google Workspace, Okta).
* Coleta de dados de vulnerabilidade e varreduras (Nessus, Qualys, OpenVAS).
* Coleta de artefatos de endpoint (hashes, processos, conexões abertas).
* Normalização dos logs para formato comum (CEF/LEEF/JSON estruturado).
* Deduplicação e compressão de eventos antes do envio.
* Correlação de logs com informações de CMDB (ativos, donos, localizações).

---

## ⚔️ **Detecção e Análise de Ameaças**

* Regras de correlação baseadas em condições lógicas (AND, OR, sequência).
* Regras comportamentais (detecção de padrões anômalos de uso).
* Regras de detecção baseadas em assinaturas (hashes, IOC, YARA, Sigma).
* Regras de detecção baseadas em estatísticas e limiares dinâmicos.
* Detecção de brute force (SSH, RDP, AD, VPN).
* Detecção de execução de comandos suspeitos (PowerShell, bash, WMI).
* Detecção de uso de contas privilegiadas fora de horário.
* Detecção de escalonamento de privilégios (sudo, UAC bypass, token abuse).
* Detecção de movimentação lateral (SMB, RDP, WinRM, SSH).
* Detecção de beaconing (comunicação periódica com C2).
* Detecção de uso de ferramentas ofensivas (Mimikatz, nmap, netcat).
* Detecção de scripts ofuscados e execução via interpretes (cmd, cscript).
* Detecção de malware por IOC (hash, domínio, IP).
* Detecção de ransomware (alterações massivas, renomeações em série).
* Detecção de exfiltração de dados (transferências anômalas, cloud storage).
* Detecção de anomalias de login geográfico (impossível travel).
* Detecção de alterações em logs e políticas de auditoria (log tampering).
* Análise de padrões MITRE ATT&CK (táticas e técnicas).
* Classificação automática de severidade (info, low, medium, high, critical).
* Enriquecimento de eventos com contexto (usuário, host, vulnerabilidade, geolocalização).

---

## 🧠 **Análise e Correlação**

* Motor de correlação em tempo real (eventos relacionados em janelas de tempo).
* Regras de correlação entre fontes distintas (ex.: firewall + AD + endpoint).
* Identificação de cadeia de ataque (kill chain).
* Correlação de alertas por entidade (usuário, IP, host).
* Reagrupamento de alertas repetitivos (alert deduplication).
* Contextualização de eventos com dados de vulnerabilidade e inventário.
* Reclassificação automática de severidade com base em impacto e confiança.
* Envio de alertas correlacionados para incidentes únicos.
* Identificação de falsos positivos com base em histórico.
* Relacionamento de eventos a campanhas conhecidas (APT, ransomware, phishing).

---

## 🚨 **Alertas e Notificações**

* Geração automática de alertas por regras de detecção.
* Thresholds configuráveis (falhas consecutivas, frequência, volume).
* Agrupamento de alertas relacionados por host ou ataque.
* Classificação por severidade e tipo (reconhecimento, execução, exfiltração).
* Notificações por e-mail, SMS, push, Slack, Teams, Telegram.
* Escalonamento automático de incidentes críticos.
* Políticas de blackout e manutenção planejada.
* Integração com ferramentas de ticket (Jira, ServiceNow, GLPI, OTRS).
* Relatórios de falsos positivos e tendências de detecção.
* SLA de resposta configurável (tempo máximo para triagem e mitigação).

---

## 🧩 **Resposta a Incidentes (SOAR)**

* Execução de playbooks automáticos por tipo de incidente.
* Ações automáticas: isolar host, bloquear IP, matar processo, revogar token.
* Execução remota de scripts (PowerShell, Bash, Python).
* Integração com firewall/EDR para bloqueio automatizado.
* Envio de IOC para bloqueio (domínios, hashes, IPs).
* Notificação de equipes e abertura de ticket automático.
* Coleta automatizada de artefatos (logs, binários, dumps).
* Execução manual de ações com aprovação (runbook guiado).
* Registro detalhado de cada ação executada (auditoria).
* Rollback de ações automáticas quando risco é mitigado.
* Acompanhamento de tempo de resposta (MTTD, MTTR).

---

## 🔐 **Gestão de Identidades e Acessos**

* Monitoramento de logons e logoffs locais e remotos.
* Detecção de logins fora do horário comercial.
* Monitoramento de tentativas de login com falha.
* Acompanhamento de criação, exclusão e alteração de usuários e grupos.
* Detecção de contas órfãs e inativas.
* Monitoramento de uso de credenciais privilegiadas.
* Verificação de políticas de senha e MFA.
* Auditoria de alterações em permissões e ACLs.
* Alertas de alteração em contas de serviço.

---

## 🧮 **Análise de Vulnerabilidades e Postura**

* Importação de resultados de varreduras (Nessus, Qualys, OpenVAS).
* Associação de vulnerabilidades a ativos monitorados.
* Correlação de vulnerabilidades com eventos de exploração.
* Priorização de vulnerabilidades com base em CVSS e exposição real.
* Rastreamento de vulnerabilidades não corrigidas.
* Relatórios de risco agregado por ativo, sistema ou cliente.
* Detecção de software desatualizado ou não autorizado.
* Monitoramento de patches de segurança pendentes.
* Conformidade com baseline de segurança (CIS, NIST, ISO).
* Avaliação de postura de endpoint (antivírus, firewall, criptografia).

---

## 🧾 **Relatórios e Dashboards**

* Painel de incidentes abertos, em investigação e resolvidos.
* Dashboard de tentativas de login e bloqueios.
* Relatórios de top usuários, hosts e IPs mais incidentes.
* Mapas geográficos de origem de ataques.
* Dashboard de regras de detecção mais acionadas.
* Métricas de tempo médio de detecção e resposta (MTTD/MTTR).
* Relatórios de vulnerabilidades críticas não mitigadas.
* Relatórios de conformidade (ISO 27001, LGPD, GDPR).
* Exportação de relatórios em PDF, CSV e API.
* Linha do tempo de campanhas e ataques.

---

## 🧭 **Governança e Conformidade**

* Registro de auditoria completo (quem visualizou, alterou ou respondeu incidentes).
* Gestão de políticas de detecção, resposta e retenção de logs.
* Retenção de logs configurável (dias, meses, anos).
* Assinatura e verificação de integridade de logs (hash, cadeia de confiança).
* Criptografia de logs em repouso e em trânsito.
* Controle de acesso baseado em papéis (analista, supervisor, admin).
* Modo multi-tenant (isolamento de clientes).
* Controle de visibilidade por cliente/contrato.
* Relatórios de conformidade e auditoria.
* Integração com SIEMs externos (Splunk, QRadar, Sentinel).
* Integração com GRC (Governance, Risk and Compliance).

---

## ☁️ **Segurança em Cloud**

* Coleta de logs de contas e permissões (IAM).
* Detecção de uso de chaves expostas e credenciais comprometidas.
* Monitoramento de buckets e storages públicos.
* Detecção de configurações inseguras (misconfigurations).
* Correlação de eventos de cloud com hosts on-premises.
* Regras de conformidade com benchmarks CIS/AWS/Azure/GCP.
* Integração com ferramentas nativas (GuardDuty, Security Center, Cloud Armor).
* Detecção de anomalias de custo (crypto mining, abuso de recursos).

---

## 🧱 **Análise Forense**

* Coleta de artefatos (arquivos, logs, dumps de memória).
* Preservação de evidências com hash e cadeia de custódia.
* Timeline de eventos e reconstrução de ataques.
* Análise de processos e conexões ativas no momento do incidente.
* Extração de IOC de logs e arquivos.
* Geração de relatórios técnicos de incidente.
* Exportação de evidências em formato forense (JSON, CSV, AFF4).

---

## 🤖 **Machine Learning e Inteligência de Ameaças (Threat Intel)**

* Enriquecimento com feeds de IOC externos (AlienVault OTX, AbuseIPDB, MISP).
* Detecção de padrões anômalos com aprendizado de máquina.
* Classificação automática de alertas baseada em histórico.
* Previsão de risco com base em tendências e comportamento.
* Identificação de novas famílias de ataque por similaridade.
* Aprendizado contínuo a partir de incidentes rotulados.
* Visualização de campanhas de ataque correlacionadas.

---

## 🧩 **Integrações e APIs**

* API REST para ingestão e exportação de eventos.
* Webhooks para notificação de novos incidentes.
* Conectores com EDR/AV (Defender, CrowdStrike, SentinelOne).
* Integração com firewalls (FortiGate, Palo Alto, Cisco ASA).
* Integração com IDS/IPS (Suricata, Zeek, Snort).
* Integração com correio eletrônico (análise de phishing).
* Integração com CMDB e inventário de ativos.
* Integração com ferramentas de resposta automatizada (SOAR).

---

## 🧮 **Análise Estatística e KPIs**

* Quantidade de eventos por hora/dia/semana.
* Tendência de alertas por severidade.
* Tempo médio de resposta por analista.
* Percentual de falsos positivos por regra.
* Taxa de reincidência de incidentes.
* Comparativo entre clientes/ambientes.
* Efetividade das ações automatizadas.

---

## 🧠 **Treinamento e Simulação**

* Modo “Simulação de ataque” (red team / purple team).
* Testes de detecção com amostras controladas (EICAR, simulações MITRE).
* Treinamento de analistas com base em incidentes simulados.
* Relatórios de cobertura de regras e gaps de detecção.

---

## 🔄 **Alta Disponibilidade e Escalabilidade**

* Balanceamento de carga entre coletores e analisadores.
* Retentativa e buffer local em caso de falha de rede.
* Replicação de dados e failover automático.
* Distribuição por zonas (multi-site, multi-tenant).
* Tolerância a falhas com reprocessamento automático de eventos.

---

## 📦 **Gerenciamento de Agentes**

* Registro e autenticação de agentes via mTLS ou token.
* Atualização automática e assinada de agentes.
* Telemetria de saúde do agente (versão, fila, consumo, uptime).
* Políticas de coleta e detecção enviadas remotamente.
* Controle remoto de ações e permissões.
* Modo manutenção (pausa temporária de coletas).

---

[⬅️ Voltar ao README](../../README.md)
