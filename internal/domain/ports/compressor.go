package ports

type Compressor interface {
	Compress(in []byte) ([]byte, error)
}
