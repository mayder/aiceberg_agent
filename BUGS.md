# BUGS.md

Registro oficial de bugs conhecidos, reprodução, impacto, correção e reteste.

## Regras

- Todo bug precisa ter severidade, área, passos de reprodução, esperado, observado e critério de fechamento.
- Bug corrigido só fecha após reteste seguindo `TESTES.md`.
- Bugs críticos ou altos bloqueiam release da frente afetada.
- Bugs de UI precisam de evidência visual quando tecnicamente possível.
- Não apagar bugs antigos; atualizar status e histórico.
- Causa provável deve separar hipótese principal de alternativas.

## Severidades

- Crítica: impede login, operação principal, pagamento, entrega, publicação ou expõe dados.
- Alta: quebra fluxo importante, causa interpretação operacional errada ou fere segurança.
- Média: degrada experiência sem bloquear fluxo principal.
- Baixa: ajuste visual, texto ou comportamento secundário.

## Diagnóstico obrigatório

1. Causa mais provável.
2. Uma ou duas hipóteses alternativas.
3. Menor correção possível.
4. Como validar.

## Validação por risco

### Bug simples

- Correção local e pequena.
- Reteste focado no passo de reprodução.
- Teste automatizado só é obrigatório se já existir cobertura próxima ou se a correção alterar regra.
- Não exige suíte completa.

### Bug complexo

- Toca regra, contrato, banco, permissão, segurança, integração, concorrência, sincronização ou vários módulos.
- Exige teste automatizado novo ou ajustado quando tecnicamente viável.
- Exige `./check.sh`.
- Exige validação completa do fluxo afetado e regressões próximas.

## Organização

Ainda não há bugs registrados.

## Modelo para novos registros

```txt
## [BUG-001] Título curto

Status:
Severidade:
Área:
Módulo:
Pacote relacionado:
Tela relacionada:
Data:

Objetivo:

Passos para reproduzir:

1.
2.
3.

Resultado esperado:

Resultado observado:

Impacto:

Causa mais provável:

Hipóteses alternativas:

Menor correção possível:

Correção aplicada:

Validação/reteste:

Evidência:

Critério de fechamento:

Validação executada:
```

## Adaptacao deste projeto

- Projeto: `aiceberg_agent`
- Tipo: Agente Go
- Stack detectada: Go agent/CLI, gopsutil, NTP, SNMP
- Regra: adaptar comandos, camadas, testes, fixtures e limites conforme a realidade deste modulo Go.
