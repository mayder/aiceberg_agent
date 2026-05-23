#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

log "validando regras essenciais"

grep -F "SOLID" QUALITY_ROADMAP.md >/dev/null || fail "QUALITY_ROADMAP.md deve declarar SOLID"
grep -F "Leitura mínima por tipo de tarefa" QUALITY_ROADMAP.md >/dev/null || fail "QUALITY_ROADMAP.md deve definir leitura mínima"
grep -F "Resposta final curta" QUALITY_ROADMAP.md >/dev/null || fail "QUALITY_ROADMAP.md deve definir resposta final curta"
grep -F 'main` ou `hml' QUALITY_ROADMAP.md >/dev/null || fail "QUALITY_ROADMAP.md deve proteger main/hml"
grep -F "Nunca usar migrations" QUALITY_ROADMAP.md >/dev/null || fail "QUALITY_ROADMAP.md deve proibir migrations"
grep -F "Migrations são proibidas" GOVERNANCA.md >/dev/null || fail "GOVERNANCA.md deve proibir migrations"
grep -F "Listagem e filtros" TELAS.md >/dev/null || fail "TELAS.md deve separar CRUD"
grep -F "Unitário" TESTES.md >/dev/null || fail "TESTES.md deve definir testes unitários"
grep -F "Cobertura mínima por criticidade" TESTES.md >/dev/null || fail "TESTES.md deve definir cobertura por criticidade"
grep -F "Dados de teste e fixtures" TESTES.md >/dev/null || fail "TESTES.md deve definir política de fixtures"
grep -F "Observabilidade mínima da aplicação" QUALITY_ROADMAP.md >/dev/null || fail "QUALITY_ROADMAP.md deve definir observabilidade mínima"
grep -F "retenção e limpeza" QUALITY_ROADMAP.md >/dev/null || fail "QUALITY_ROADMAP.md deve exigir retenção e limpeza"
grep -F "Review de fechamento de pacote" QUALITY_ROADMAP.md >/dev/null || fail "QUALITY_ROADMAP.md deve definir review de fechamento"
grep -F "Contrato de módulo" QUALITY_ROADMAP.md >/dev/null || fail "QUALITY_ROADMAP.md deve definir contrato de módulo"
grep -F "Nomenclatura oficial do projeto" QUALITY_ROADMAP.md >/dev/null || fail "QUALITY_ROADMAP.md deve definir nomenclatura oficial"
grep -F "Nomenclatura oficial do projeto" ESCOPO.md >/dev/null || fail "ESCOPO.md deve conter nomenclatura oficial"
grep -F "Adaptação à arquitetura real" QUALITY_ROADMAP.md >/dev/null || fail "QUALITY_ROADMAP.md deve definir adaptação à arquitetura real"
grep -F "DECISOES.md" QUALITY_ROADMAP.md >/dev/null || fail "QUALITY_ROADMAP.md deve apontar decisões para DECISOES.md"
grep -F "DEC-YYYYMMDD-01" DECISOES.md >/dev/null || fail "DECISOES.md deve ter modelo de decisão"
grep -F "Check adaptável por stack" QUALITY_ROADMAP.md >/dev/null || fail "QUALITY_ROADMAP.md deve definir check por stack"
grep -F "Quando criar pacote" DEMANDAS.md >/dev/null || fail "DEMANDAS.md deve definir quando criar pacote"
