package ports

// Compressor aplica compactação no payload bruto.
type Compressor interface {
	Compress(in []byte) ([]byte, error)
}
