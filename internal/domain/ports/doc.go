// Package ports define contratos de integração usados pelo domínio.
//
// Regras gerais:
// - Implementações devem ser seguras para concorrência ou documentar o contrário.
// - Erros devem ser retornados (sem panics) e com contexto suficiente.
// - As portas não devem logar; o domínio/uso de casos controla observabilidade.
package ports
