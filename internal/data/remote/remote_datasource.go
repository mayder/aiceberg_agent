package remote

// IngestClient abstrai o cliente HTTP (ou futuro gRPC) que envia os lotes.
type IngestClient interface {
	// SendBatch envia um corpo (geralmente JSON) para o endpoint de ingestão.
	// Retorna status HTTP, corpo de resposta e erro de transporte.
	SendBatch(endpoint string, body []byte, headers map[string]string) (status int, resp []byte, err error)
}
