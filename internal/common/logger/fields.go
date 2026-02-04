package logger

import (
	"fmt"
	"strings"
)

// KV monta uma mensagem com campos "chave=valor" no padrão de logs do agente.
// Ignora pares com chave vazia, valor nil ou string vazia.
func KV(msg string, kv ...any) string {
	fields := Fields(kv...)
	if fields == "" {
		return msg
	}
	return msg + " " + fields
}

// Fields renderiza pares chave/valor em "chave=valor".
// Ignora pares com chave vazia, valor nil ou string vazia.
func Fields(kv ...any) string {
	if len(kv) == 0 {
		return ""
	}
	var b strings.Builder
	first := true
	for i := 0; i+1 < len(kv); i += 2 {
		key, ok := kv[i].(string)
		if !ok || key == "" {
			continue
		}
		val := kv[i+1]
		if val == nil {
			continue
		}
		if s, ok := val.(string); ok && s == "" {
			continue
		}
		if !first {
			b.WriteByte(' ')
		}
		first = false
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(fmt.Sprint(val))
	}
	return b.String()
}
