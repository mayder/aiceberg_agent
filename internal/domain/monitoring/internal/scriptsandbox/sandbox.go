package scriptsandbox

import (
	"context"
	"errors"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

const invocationLimit = 50 * time.Millisecond

var (
	ErrInvalid    = errors.New("pipeline_script_invalid")
	ErrBusy       = errors.New("pipeline_script_busy")
	ErrTimeout    = errors.New("pipeline_script_timeout")
	ErrTrap       = errors.New("pipeline_script_trap")
	ErrValueLimit = errors.New("pipeline_script_value_limit")
	slots         = make(chan struct{}, 4)
)

type Program struct {
	raw  []byte
	cost int
}

func (p Program) Cost() int { return p.cost }

func Compile(raw []byte) (Program, error) {
	cost, ok := profile(raw)
	if !ok {
		return Program{}, ErrInvalid
	}
	if !acquire() {
		return Program{}, ErrBusy
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), invocationLimit)
	defer cancel()
	runtime := newRuntime(ctx)
	defer runtime.Close(context.Background())
	if _, err := runtime.CompileModule(ctx, raw); err != nil {
		return Program{}, sandboxError(ctx, ErrInvalid)
	}
	if ctx.Err() != nil {
		return Program{}, ErrTimeout
	}
	return Program{raw: append([]byte(nil), raw...), cost: cost}, nil
}

func (p Program) Run(parent context.Context, input []byte) ([]byte, error) {
	if parent.Err() != nil {
		return nil, ErrTimeout
	}
	if p.cost == 0 {
		return nil, ErrInvalid
	}
	if len(input) == 0 || len(input) > MaxValueBytes {
		return nil, ErrValueLimit
	}
	if !acquire() {
		return nil, ErrBusy
	}
	defer release()
	ctx, cancel := context.WithTimeout(parent, invocationLimit)
	defer cancel()
	runtime := newRuntime(ctx)
	defer runtime.Close(context.Background())
	module, err := instantiate(ctx, runtime, p.raw)
	if err != nil {
		return nil, err
	}
	return transform(ctx, module, input)
}

func newRuntime(ctx context.Context) wazero.Runtime {
	config := wazero.NewRuntimeConfigInterpreter().
		WithCoreFeatures(api.CoreFeaturesV1).
		WithMemoryLimitPages(1).
		WithCloseOnContextDone(true).
		WithDebugInfoEnabled(false)
	return wazero.NewRuntimeWithConfig(ctx, config)
}

func instantiate(ctx context.Context, runtime wazero.Runtime, raw []byte) (api.Module, error) {
	compiled, err := runtime.CompileModule(ctx, raw)
	if err != nil {
		return nil, sandboxError(ctx, ErrInvalid)
	}
	module, err := runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("").WithStartFunctions())
	if err != nil {
		return nil, sandboxError(ctx, ErrTrap)
	}
	return module, nil
}

func transform(ctx context.Context, module api.Module, input []byte) ([]byte, error) {
	memory := module.ExportedMemory("memory")
	entry := module.ExportedFunction("transform")
	if memory == nil || entry == nil || !memory.Write(0, input) {
		return nil, sandboxError(ctx, ErrTrap)
	}
	result, err := entry.Call(ctx, uint64(len(input)))
	if err != nil || len(result) != 1 {
		return nil, sandboxError(ctx, ErrTrap)
	}
	if ctx.Err() != nil {
		return nil, ErrTimeout
	}
	size := uint32(result[0])
	if size == 0 || size > MaxValueBytes {
		return nil, ErrValueLimit
	}
	raw, ok := memory.Read(0, size)
	if !ok {
		return nil, ErrValueLimit
	}
	return append([]byte(nil), raw...), nil
}

func sandboxError(ctx context.Context, fallback error) error {
	if ctx.Err() != nil {
		return ErrTimeout
	}
	return fallback
}

func acquire() bool {
	select {
	case slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func release() { <-slots }
