---
title: Modelo executivo de desenvolvimento com IA
markmap:
  colorFreezeLevel: 2
  maxWidth: 280
---

# Desenvolvimento com IA

## Objetivo

### Menos tokens

- Ler `PATHS.toml`
- Identificar tipo da tarefa
- Ler só o necessário
- Usar playbook certo

### Mais qualidade

- SOLID
- Boundaries reais
- Teste proporcional
- Check automatizado
- Review antes do commit

### Menos risco

- Branch conferida
- `main` e `hml` exigem confirmação
- Pacote fechado com commit
- Rollback documentado

## Ciclo principal

### 0. Ler pouco

- `PATHS.toml`
- Tipo da tarefa
- Arquivos mínimos
- Ampliar leitura só se houver risco

### 1. Orientar

- Inspecionar projeto real
- Mapear linguagem e framework
- Preservar convenções saudáveis
- Documentar arquitetura real

### 2. Planejar

- Usar `DEMANDAS.md`
- Separar pacote e lote
- Definir escopo
- Definir testes esperados

### 3. Implementar

- Um lote por vez
- Separar responsabilidades
- Evitar arquivo grande
- Registrar decisão relevante

### 4. Validar

- Lote: teste raso
- Pacote: teste completo
- Bug simples: reteste focado
- Bug complexo: validação completa

### 5. Fechar

- Review de fechamento
- Docs sincronizadas
- `./check.sh`
- Commit do pacote
- Push opcional
- Resposta curta

## Regras centrais

### SOLID

- Responsabilidade única
- Extensão sem alteração arriscada
- Contratos substituíveis
- Interfaces pequenas
- Domínio sem depender de infra

### CRUD

- Listagem e filtros
- Detalhamento
- Cadastro
- Edição
- Formulário compartilhado sem regra de rota

### Testes

- Unitário
- Service/use case
- Contrato/API
- Componente/widget
- E2E só quando faz sentido
- Fixtures controladas

### Operação

- Logs seguros
- Auditoria quando necessário
- Correlação
- Retenção e cleanup
- Evitar tabela nova sem justificativa

### Check

- Scripts pequenos
- Stack configurável
- Camadas configuráveis
- Fixtures configuráveis
- Runtime sem nomes internos

## Arquivos principais

### Direção

- `PATHS.toml`
- `ESCOPO.md`
- `QUALITY_ROADMAP.md`
- `GOVERNANCA.md`

### Execução

- `DEMANDAS.md`
- `TELAS.md`
- `TESTES.md`
- `BUGS.md`

### Operação

- `DECISOES.md`
- `RUNBOOK.md`
- `check.sh`
- `scripts/`

## Resultado esperado

- IA com menos ambiguidade
- Código menor
- Módulos claros
- Testes úteis
- Pacotes fechados limpos
- Manutenção viável

## Adaptacao deste projeto

- Projeto: `aiceberg_agent`
- Tipo: Agente Go
- Stack detectada: Go agent/CLI, gopsutil, NTP, SNMP
- Regra: adaptar comandos, camadas, testes, fixtures e limites conforme a realidade deste modulo Go.
