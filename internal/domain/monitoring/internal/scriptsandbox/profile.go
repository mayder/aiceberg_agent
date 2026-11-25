package scriptsandbox

import (
	"bytes"
	"encoding/binary"
)

const (
	MaxModuleBytes  = 8192
	MaxValueBytes   = 8192
	maxInstructions = 512
	maxDepth        = 16
	maxLocals       = 32
)

var (
	header         = []byte{0, 'a', 's', 'm', 1, 0, 0, 0}
	functionType   = []byte{1, 0x60, 1, 0x7f, 1, 0x7f} // (i32) -> i32
	functionList   = []byte{1, 0}                      // One function using type 0.
	memoryLimits   = []byte{1, 1, 1, 1}                // One memory with min=max=1 page.
	memoryExport   = []byte{6, 'm', 'e', 'm', 'o', 'r', 'y', 2, 0}
	functionExport = []byte{9, 't', 'r', 'a', 'n', 's', 'f', 'o', 'r', 'm', 0, 0}
)

// profile accepts an intentionally narrow Core WebAssembly profile before
// wazero decodes the module. This bounds vectors and rejects host capabilities.
func profile(raw []byte) (int, bool) {
	if len(raw) > MaxModuleBytes || !bytes.HasPrefix(raw, header) {
		return 0, false
	}
	body := raw[len(header):]
	for _, id := range []byte{1, 3, 5, 7, 10} {
		if len(body) == 0 || body[0] != id {
			return 0, false
		}
		size, n := binary.Uvarint(body[1:])
		if n < 1 || size > uint64(len(body)-1-n) {
			return 0, false
		}
		section := body[1+n : 1+n+int(size)]
		body = body[1+n+int(size):]
		if id == 10 {
			if len(body) != 0 {
				return 0, false
			}
			return profileCode(section)
		}
		if !profileSection(id, section) {
			return 0, false
		}
	}
	return 0, false
}

func profileSection(id byte, section []byte) bool {
	switch id {
	case 1:
		return bytes.Equal(section, functionType)
	case 3:
		return bytes.Equal(section, functionList)
	case 5:
		return bytes.Equal(section, memoryLimits)
	case 7:
		if len(section) != 1+len(memoryExport)+len(functionExport) || section[0] != 2 {
			return false
		}
		exports := section[1:]
		return (bytes.HasPrefix(exports, memoryExport) && bytes.HasSuffix(exports, functionExport)) ||
			(bytes.HasPrefix(exports, functionExport) && bytes.HasSuffix(exports, memoryExport))
	}
	return false
}

func profileCode(section []byte) (int, bool) {
	if len(section) < 3 || section[0] != 1 {
		return 0, false
	}
	size, n := binary.Uvarint(section[1:])
	if n < 1 || size != uint64(len(section)-1-n) {
		return 0, false
	}
	body := section[1+n:]
	code, ok := profileLocals(body)
	if !ok {
		return 0, false
	}
	return profileInstructions(code)
}

func profileLocals(raw []byte) ([]byte, bool) {
	groups, n := binary.Uvarint(raw)
	if n < 1 || groups > maxLocals {
		return nil, false
	}
	raw = raw[n:]
	total := uint64(0)
	for i := uint64(0); i < groups; i++ {
		count, n := binary.Uvarint(raw)
		if n < 1 || n >= len(raw) || count > maxLocals-total || raw[n] < 0x7c || raw[n] > 0x7f {
			return nil, false
		}
		total += count
		raw = raw[n+1:]
	}
	return raw, true
}

func profileInstructions(raw []byte) (int, bool) {
	depth := 1
	for count := 1; count <= maxInstructions && len(raw) != 0; count++ {
		op := raw[0]
		raw = raw[1:]
		switch op {
		case 0x02, 0x04: // block, if; no backward branches.
			depth++
			if depth > maxDepth {
				return 0, false
			}
		case 0x0b: // end
			depth--
			if depth == 0 {
				return count, len(raw) == 0
			}
		}
		n, ok := immediateSize(op, raw)
		if !ok || n > len(raw) {
			return 0, false
		}
		raw = raw[n:]
	}
	return 0, false
}

func immediateSize(op byte, raw []byte) (int, bool) {
	switch {
	case op == 0x02 || op == 0x04: // single-byte block value type.
		return 1, len(raw) != 0 && (raw[0] == 0x40 || (raw[0] >= 0x7c && raw[0] <= 0x7f))
	case op == 0x0c || op == 0x0d || (op >= 0x20 && op <= 0x22) || op == 0x41:
		n := lebSize(raw, 5)
		return n, n != 0
	case op == 0x42:
		n := lebSize(raw, 10)
		return n, n != 0
	case op == 0x43:
		return 4, true
	case op == 0x44:
		return 8, true
	case op >= 0x28 && op <= 0x3e: // memory load/store alignment and offset.
		a := lebSize(raw, 5)
		b := lebSize(raw[a:], 5)
		return a + b, a != 0 && b != 0
	case op == 0 || op == 1 || op == 5 || op == 0x0b || op == 0x0f || op == 0x1a || op == 0x1b,
		op >= 0x45 && op <= 0xbf: // Core scalar arithmetic, comparison and conversion.
		return 0, true
	}
	return 0, false
}

func lebSize(raw []byte, maximum int) int {
	for i := 0; i < min(len(raw), maximum); i++ {
		if raw[i]&0x80 == 0 {
			return i + 1
		}
	}
	return 0
}
