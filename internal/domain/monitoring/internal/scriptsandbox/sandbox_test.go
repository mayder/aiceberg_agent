package scriptsandbox

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"
)

func section(id byte, contents []byte) []byte {
	out := binary.AppendUvarint([]byte{id}, uint64(len(contents)))
	return append(out, contents...)
}

func testModule(code []byte) []byte {
	body := append([]byte{0}, code...)
	sectionCode := binary.AppendUvarint([]byte{1}, uint64(len(body)))
	sectionCode = append(sectionCode, body...)
	raw := bytes.Clone(header)
	raw = append(raw, section(1, functionType)...)
	raw = append(raw, section(3, functionList)...)
	raw = append(raw, section(5, memoryLimits)...)
	exports := append(append([]byte{2}, memoryExport...), functionExport...)
	raw = append(raw, section(7, exports)...)
	return append(raw, section(10, sectionCode)...)
}

func compileForTest(t *testing.T, code []byte) Program {
	t.Helper()
	p, err := Compile(testModule(code))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSandboxReturnsTypedBytesWithFreshMemoryOnEveryInvocation(t *testing.T) {
	// Output is byte '0' plus the previous value of memory[32]. Write 1
	// to memory[32]; it must disappear when the invocation is closed.
	program := compileForTest(t, []byte{
		0x41, 0, 0x41, 32, 0x2d, 0, 0, 0x41, 48, 0x6a, 0x3a, 0, 0,
		0x41, 32, 0x41, 1, 0x3a, 0, 0, 0x41, 1, 0x0b,
	})
	for i := 0; i < 2; i++ {
		got, err := program.Run(context.Background(), []byte("5"))
		if err != nil || string(got) != "0" {
			t.Fatal("unexpected result or persistent guest memory", err)
		}
	}
}

func TestSandboxProfileRejectsHostFeaturesAndUnboundedInstructions(t *testing.T) {
	identity := []byte{0x20, 0, 0x0b}
	valid := testModule(identity)
	for name, raw := range map[string][]byte{
		"import section":  append(append(bytes.Clone(header), section(2, []byte{0})...), valid[len(header):]...),
		"start section":   append(bytes.Clone(valid), section(8, []byte{0})...),
		"too much memory": bytes.Replace(valid, section(5, memoryLimits), section(5, []byte{1, 1, 1, 2}), 1),
		"too many locals": bytes.Replace(valid, section(10, []byte{1, 4, 0, 0x20, 0, 0x0b}),
			section(10, []byte{1, 6, 1, 33, 0x7f, 0x20, 0, 0x0b}), 1),
		"loop":                               testModule([]byte{0x03, 0x40, 0x0b, 0x20, 0, 0x0b}),
		"call":                               testModule([]byte{0x10, 0, 0x0b}),
		"grow":                               testModule([]byte{0x20, 0, 0x40, 0, 0x0b}),
		"too many instructions":              testModule(append(bytes.Repeat([]byte{1}, maxInstructions-1), identity...)),
		"too large":                          append(bytes.Clone(valid), make([]byte, MaxModuleBytes)...),
		"valid byte shape but invalid stack": testModule([]byte{0x6a, 0x0b}),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Compile(raw); !errors.Is(err, ErrInvalid) {
				t.Fatal("unexpected admission", err)
			}
		})
	}
	bounded := compileForTest(t, append(bytes.Repeat([]byte{1}, maxInstructions-2), identity...))
	if bounded.Cost() != maxInstructions {
		t.Fatal("incorrect instruction charge", bounded.Cost())
	}
}

func TestSandboxEnforcesValueBoundsAndContainsGuestTrap(t *testing.T) {
	for name, tc := range map[string]struct {
		code []byte
		want error
	}{
		"trap":            {[]byte{0, 0x0b}, ErrTrap},
		"output limit":    {[]byte{0x41, 0x81, 0xc0, 0, 0x0b}, ErrValueLimit},
		"negative length": {[]byte{0x41, 0x7f, 0x0b}, ErrValueLimit},
	} {
		t.Run(name, func(t *testing.T) {
			p := compileForTest(t, tc.code)
			if out, err := p.Run(context.Background(), []byte("1")); !errors.Is(err, tc.want) || out != nil {
				t.Fatal("failed program returned a value", err)
			}
		})
	}
	p := compileForTest(t, []byte{0x20, 0, 0x0b})
	input := bytes.Repeat([]byte("1"), MaxValueBytes)
	if out, err := p.Run(context.Background(), input); err != nil || !bytes.Equal(out, input) {
		t.Fatal("exact bound did not work", err)
	}
	if _, err := p.Run(context.Background(), append(input, '1')); err != ErrValueLimit {
		t.Fatal("oversize input accepted", err)
	}
}

func TestSandboxRejectsCancelledAndOverCapacityInvocations(t *testing.T) {
	p := compileForTest(t, []byte{0x20, 0, 0x0b})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if out, err := p.Run(ctx, []byte("0")); out != nil || err != ErrTimeout {
		t.Fatal("canceled call returned a value", err)
	}
	for i := 0; i < cap(slots); i++ {
		if !acquire() {
			t.Fatal("slot not available")
		}
		defer release()
	}
	if out, err := p.Run(context.Background(), []byte("0")); out != nil || err != ErrBusy {
		t.Fatal("over-capacity script ran", err)
	}
}

func FuzzSandboxProfile(f *testing.F) {
	f.Add(testModule([]byte{0x20, 0, 0x0b}))
	f.Add(testModule([]byte{0x03, 0x40, 0x0b}))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, raw []byte) {
		cost, valid := profile(raw)
		if valid && (cost < 1 || cost > maxInstructions) {
			t.Fatal("admission escaped instruction bounds")
		}
	})
}
