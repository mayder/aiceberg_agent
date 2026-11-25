package agentless

import (
	"context"
	"errors"
	"net"
	"strings"
)

type snmpFailure struct {
	code    string
	message string
	detail  string
}

func classifySNMPFailure(text string) snmpFailure {
	text = strings.ToLower(text)
	switch {
	case strings.Contains(text, "timeout"), strings.Contains(text, "no response"), strings.Contains(text, "deadline exceeded"):
		return snmpFailure{"snmp_timeout", "SNMP sem resposta (timeout)", "timeout"}
	case strings.Contains(text, "authentication"), strings.Contains(text, "authorization"), strings.Contains(text, "unknown username"),
		strings.Contains(text, "wrongdigest"), strings.Contains(text, "wrong digest"), strings.Contains(text, "decryption"):
		return snmpFailure{"snmp_auth", "Falha de credencial ou versão SNMP", "authentication failure"}
	case strings.Contains(text, "connection refused"):
		return snmpFailure{"snmp_refused", "Conexão SNMP recusada", "connection refused"}
	case strings.Contains(text, "tabela sem retorno"):
		return snmpFailure{"snmp_table_empty", "Tabela SNMP sem retorno", "tabela sem retorno"}
	default:
		return snmpFailure{}
	}
}

// A library error can embed a peer address, username or raw packet details.
// Preserve diagnostic categories while keeping the original error in memory.
func safeSNMPError(err error) string {
	var networkError net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return "consulta cancelada"
	case errors.Is(err, errSNMPBudgetExceeded):
		return "budget excedido"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &networkError) && networkError.Timeout():
		return "timeout"
	}
	if detail := classifySNMPFailure(err.Error()).detail; detail != "" {
		return detail
	}
	if isSNMPUnsupportedError(err) {
		return "no such object"
	}
	return "falha de comunicação SNMP"
}
