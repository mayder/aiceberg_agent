package local

// OutboxDataSource abstrai a fila local (WAL) onde guardamos os envelopes
// quando offline ou antes do envio (ex.: bbolt implementation).
type OutboxDataSource interface {
	Append(topic string, payload []byte) error
	ReadBatch(maxBytes int) (keys [][]byte, payloads [][]byte, err error)
	Commit(keys [][]byte) error
	Len() (items int, bytes int64)
	Close() error
}
