package transport

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func buildRequest(url string, batch []entities.Envelope, cfg config.Config) (*http.Request, error) {
	body, err := json.Marshal(batch)
	if err != nil {
		return nil, err
	}

	rdr := bytes.NewReader(body)
	if cfg.HTTPGzip {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(body); err != nil {
			return nil, err
		}
		_ = gz.Close()
		rdr = bytes.NewReader(buf.Bytes())
	}

	req, err := http.NewRequest(http.MethodPost, url, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.HTTPGzip {
		req.Header.Set("Content-Encoding", "gzip")
	}
	if cfg.HTTPIdempotency {
		req.Header.Set("Idempotency-Key", newIdempotencyKey())
	}
	return req, nil
}

func newIdempotencyKey() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}
