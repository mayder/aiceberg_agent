package ports

import "github.com/you/aiceberg_agent/internal/domain/entities"

type Transport interface {
	// SendWithAuth envia um batch aplicando o header Authorization fornecido (se não vazio).
	// Retorna o body da resposta (se houver).
	SendWithAuth(batch []entities.Envelope, authHeader string, endpoint string) ([]byte, error)
}
