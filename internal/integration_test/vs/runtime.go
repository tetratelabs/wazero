package vs

import (
	"context"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/wasm"
)

type RuntimeConfig struct {
	Name              string
	ModuleName        string
	ModuleWasm        []byte
	FuncNames         []string
	NeedsWASI         bool
	NeedsMemoryExport bool
	// LogFn requires the implementation to export a function "env.log" which accepts i32i32_v.
	// The implementation invoke this with a byte slice allocated from the offset, length pair.
	// This function simulates a host function that logs a message.
	LogFn func([]byte) error
	// EnvFReturnValue is set to non-zero if we want the runtime to instantiate "env" module with the function "f"
	// which accepts one i64 value and returns the EnvFReturnValue as i64. This is mutually exclusive to LogFn.
	EnvFReturnValue uint64
}

type Runtime interface {
	Name() string
	Compile(context.Context, *RuntimeConfig) error
	Instantiate(context.Context, *RuntimeConfig) (Module, error)
	Close(context.Context) error
}

type Module interface {
	CallI32_I32(ctx context.Context, funcName string, param uint32) (uint32, error)
	CallI32I32_V(ctx context.Context, funcName string, x, y uint32) error
	CallI32_V(ctx context.Context, funcName string, param uint32) error
	CallV_V(ctx context.Context, funcName string) error
	CallI64_I64(ctx context.Context, funcName string, param uint64) (uint64, error)
	WriteMemory(offset uint32, bytes []byte) error
	Memory() []byte
	Close(context.Context) error
}

func NewWazeroInterpreterRuntime() Runtime {
	return newWazeroRuntime("wazero-interpreter", wazero.NewRuntimeConfigInterpreter())
}

func NewWazeroCompilerRuntime() Runtime {
	return newWazeroRuntime(compilerRuntime, wazero.NewRuntimeConfigCompiler())
}

func newWazeroRuntime(name string, config wazero.RuntimeConfig) *wazeroRuntime {
	return &wazeroRuntime{name: name, config: config}
}

type wazeroRuntime struct {
	name          string
	config        wazero.RuntimeConfig
	runtime       wazero.Runtime
	logFn         func([]byte) error
	env, compiled wazero.CompiledModule
}

type wazeroModule struct {
	wasi     api.Closer
	env, mod api.Module
	funcs    map[string]api.Function
}

func (r *wazeroRuntime) Name() string {
	return r.name
}

func (m *wazeroModule) Memory() []byte {
	return m.mod.Memory().(*wasm.MemoryInstance).Buffer
}

func (r *wazeroRuntime) log(_ context.Context, mod api.Module, stack []uint64) {
	offset, byteCount := uint32(stack[0]), uint32(stack[1])

	buf, ok := mod.Memory().Read(offset, byteCount)
	if !ok {
		panic("out of memory reading log buffer")
	}
	if err := r.logFn(buf); err != nil {
		panic(err)
	}
}

func (r *wazeroRuntime) Compile(ctx context.Context, cfg *RuntimeConfig) (err error) {
	r.runtime = wazero.NewRuntimeWithConfig(ctx, r.config)
	if cfg.LogFn != nil {
		r.logFn = cfg.LogFn
		if r.env, err = r.runtime.NewHostModuleBuilder("env").
			NewFunctionBuilder().
			WithGoModuleFunction(api.GoModuleFunc(r.log), []api.ValueType{api.ValueTypeI32, api.ValueTypeI32}, []api.ValueType{}).
			Export("log").
			Compile(ctx); err != nil {
			return err
		}
	} else if cfg.EnvFReturnValue != 0 {
		if r.env, err = r.runtime.NewHostModuleBuilder("env").
			NewFunctionBuilder().
			WithGoFunction(api.GoFunc(func(ctx context.Context, stack []uint64) {
				stack[0] = cfg.EnvFReturnValue
			}), []api.ValueType{api.ValueTypeI64}, []api.ValueType{api.ValueTypeI64}).
			Export("f").
			Compile(ctx); err != nil {
			return err
		}
	}
	r.compiled, err = r.runtime.CompileModule(ctx, cfg.ModuleWasm)
	return
}

func (r *wazeroRuntime) Instantiate(ctx context.Context, cfg *RuntimeConfig) (mod Module, err error) {
	wazeroCfg := wazero.NewModuleConfig().WithName(cfg.ModuleName)
	m := &wazeroModule{funcs: map[string]api.Function{}}

	// Instantiate WASI, if configured.
	if cfg.NeedsWASI {
		if m.wasi, err = wasi_snapshot_preview1.Instantiate(ctx, r.runtime); err != nil {
			return
		}
	}

	// Instantiate the host module, "env", if configured.
	if env := r.env; env != nil {
		if m.env, err = r.runtime.InstantiateModule(ctx, env, wazero.NewModuleConfig()); err != nil {
			return
		}
	}

	// Instantiate the module.
	if m.mod, err = r.runtime.InstantiateModule(ctx, r.compiled, wazeroCfg); err != nil {
		return
	}

	// Ensure function exports exist.
	for _, funcName := range cfg.FuncNames {
		if fn := m.mod.ExportedFunction(funcName); fn == nil {
			return nil, fmt.Errorf("%s is not an exported function", funcName)
		} else {
			m.funcs[funcName] = fn
		}
	}
	mod = m
	return
}

func (r *wazeroRuntime) Close(ctx context.Context) (err error) {
	if compiled := r.compiled; compiled != nil {
		err = compiled.Close(ctx)
	}
	r.compiled = nil
	if env := r.env; env != nil {
		err = env.Close(ctx)
	}
	r.env = nil
	return
}

func (m *wazeroModule) CallV_V(ctx context.Context, funcName string) (err error) {
	_, err = m.funcs[funcName].Call(ctx)
	return
}

func (m *wazeroModule) CallI32_I32(ctx context.Context, funcName string, param uint32) (uint32, error) {
	if results, err := m.funcs[funcName].Call(ctx, uint64(param)); err != nil {
		return 0, err
	} else if len(results) > 0 {
		return uint32(results[0]), nil
	}
	return 0, nil
}

func (m *wazeroModule) CallI32I32_V(ctx context.Context, funcName string, x, y uint32) (err error) {
	_, err = m.funcs[funcName].Call(ctx, uint64(x), uint64(y))
	return
}

func (m *wazeroModule) CallI32_V(ctx context.Context, funcName string, param uint32) (err error) {
	_, err = m.funcs[funcName].Call(ctx, uint64(param))
	return
}

func (m *wazeroModule) CallI64_I64(ctx context.Context, funcName string, param uint64) (uint64, error) {
	if results, err := m.funcs[funcName].Call(ctx, param); err != nil {
		return 0, err
	} else if len(results) > 0 {
		return results[0], nil
	}
	return 0, nil
}

func (m *wazeroModule) WriteMemory(offset uint32, bytes []byte) error {
	if !m.mod.Memory().Write(offset, bytes) {
		return errors.New("out of memory writing name")
	}
	return nil
}

func (m *wazeroModule) Close(ctx context.Context) (err error) {
	if mod := m.mod; mod != nil {
		err = mod.Close(ctx)
	}
	m.mod = nil
	if env := m.env; env != nil {
		err = env.Close(ctx)
	}
	m.env = nil
	if wasi := m.wasi; wasi != nil {
		err = wasi.Close(ctx)
	}
	m.wasi = nil
	return
}
