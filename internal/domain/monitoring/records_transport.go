package monitoring

import (
	"bytes"
	"errors"
	"io"
)

// EncodeTransportRecords validates compatibility before returning any wire bytes.
// It neither authorizes a route nor enqueues, splits, drops or recollects data.
func EncodeTransportRecords(b RecordsBatch, d Definition, a DefinitionAuthority, now int64, hops []TransportProfile) ([]byte, error) {
	p, err := NegotiateRecordsTransport(hops)
	if err != nil {
		return nil, err
	}
	raw, err := EncodeRecords(b, d, a, now)
	if err != nil {
		return nil, err
	}
	if err = validateTransportBatch(b, d, p); err != nil {
		return nil, err
	}
	if len(raw) > p.MaxBytes {
		return nil, errors.New("monitoring_transport_limit")
	}
	return raw, nil
}

// ReadTransportRecords limits decompressed bytes before JSON parsing/allocation.
// The transport layer owns auth, Content-Encoding, cancellation and ACK semantics.
func ReadTransportRecords(reader io.Reader, d Definition, a DefinitionAuthority, now int64, hops []TransportProfile) (RecordsBatch, error) {
	p, err := NegotiateRecordsTransport(hops)
	if err != nil {
		return RecordsBatch{}, err
	}
	if reader == nil {
		return RecordsBatch{}, errors.New("invalid_monitoring_transport_body")
	}
	raw, err := io.ReadAll(io.LimitReader(reader, int64(p.MaxBytes)+1))
	if err != nil {
		return RecordsBatch{}, errors.New("invalid_monitoring_transport_body")
	}
	if len(raw) > p.MaxBytes {
		return RecordsBatch{}, errors.New("monitoring_transport_limit")
	}
	b, err := DecodeRecords(raw)
	if err == nil {
		err = ValidateRecords(b, d, a, now)
	}
	if err == nil {
		err = validateTransportBatch(b, d, p)
	}
	if err != nil {
		return RecordsBatch{}, err
	}
	return b, nil
}

func DecodeTransportRecords(raw []byte, d Definition, a DefinitionAuthority, now int64, hops []TransportProfile) (RecordsBatch, error) {
	return ReadTransportRecords(bytes.NewReader(raw), d, a, now, hops)
}
