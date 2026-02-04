package ports

import "github.com/you/aiceberg_agent/internal/domain/entities"

// Transport envia batches para o backend.
// Deve retornar erro para status >= 300 e preservar a ordem do batch enviado.
type Transport interface {
	// SendWithAuth envia um batch aplicando o header Authorization fornecido (se não vazio).
	// Retorna o body da resposta (se houver).
	SendWithAuth(batch []entities.Envelope, authHeader string, endpoint string) ([]byte, error)
}
