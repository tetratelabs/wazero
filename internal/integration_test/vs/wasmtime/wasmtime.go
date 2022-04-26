//go:build amd64 && cgo

package wasmtime

import (
	"context"
	"fmt"

	"github.com/bytecodealliance/wasmtime-go"

	"github.com/tetratelabs/wazero/internal/integration_test/vs"
)

func newWasmtimeRuntime() vs.Runtime {
	return &wasmtimeRuntime{}
}

type wasmtimeRuntime struct {
	engine *wasmtime.Engine
}

type wasmtimeModule struct {
	store *wasmtime.Store
	// instance is here because there's no close/destroy function. The only thing is garbage collection.
	instance *wasmtime.Instance
	funcs    map[string]*wasmtime.Func
	logFn    func([]byte) error
	mem      *wasmtime.Memory
}

func (r *wasmtimeRuntime) Name() string {
	return "wasmtime"
}

func (m *wasmtimeModule) log(_ *wasmtime.Caller, args []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap) {
	unsafeSlice := m.mem.UnsafeData(m.store)
	offset := args[0].I32()
	byteCount := args[1].I32()
	if err := m.logFn(unsafeSlice[offset : offset+byteCount]); err != nil {
		return nil, wasmtime.NewTrap(err.Error())
	}
	return []wasmtime.Val{}, nil
}

func (r *wasmtimeRuntime) Compile(_ context.Context, _ *vs.RuntimeConfig) (err error) {
	r.engine = wasmtime.NewEngine()
	// We can't reuse a store because even if we call close, re-instantiating too many times leads to:
	// >> resource limit exceeded: instance count too high at 10001
	return
}

func (r *wasmtimeRuntime) Instantiate(_ context.Context, cfg *vs.RuntimeConfig) (mod vs.Module, err error) {
	wm := &wasmtimeModule{funcs: map[string]*wasmtime.Func{}}
	wm.store = wasmtime.NewStore(r.engine)
	var m *wasmtime.Module
	if m, err = wasmtime.NewModule(wm.store.Engine, cfg.ModuleWasm); err != nil {
		return
	}

	linker := wasmtime.NewLinker(wm.store.Engine)

	// Instantiate WASI, if configured.
	if cfg.NeedsWASI {
		if err = linker.DefineWasi(); err != nil {
			return
		}
		wm.store.SetWasi(wasmtime.NewWasiConfig())
	}

	// Instantiate the host module, "env", if configured.
	if cfg.LogFn != nil {
		wm.logFn = cfg.LogFn
		if err = linker.Define("env", "log", wasmtime.NewFunc(
			wm.store,
			wasmtime.NewFuncType(
				[]*wasmtime.ValType{
					wasmtime.NewValType(wasmtime.KindI32),
					wasmtime.NewValType(wasmtime.KindI32),
				},
				[]*wasmtime.ValType{},
			),
			wm.log,
		)); err != nil {
			return
		}
	}

	// Set the module name.
	if err = linker.DefineModule(wm.store, cfg.ModuleName, m); err != nil {
		return
	}

	// Instantiate the module.
	instance, err := linker.Instantiate(wm.store, m)
	if err != nil {
		return
	}

	// Wasmtime does not allow a host function parameter for memory, so you have to manually propagate it.
	if cfg.LogFn != nil {
		if wm.mem = instance.GetExport(wm.store, "memory").Memory(); wm.mem == nil {
			err = fmt.Errorf(`"memory" not exported`)
			return
		}
	}

	// If WASI is needed, we have to go back and invoke the _start function.
	if cfg.NeedsWASI {
		start := instance.GetFunc(wm.store, "_start")
		if _, err = start.Call(wm.store); err != nil {
			return
		}
	}

	// Ensure function exports exist.
	for _, funcName := range cfg.FuncNames {
		if fn := instance.GetFunc(wm.store, funcName); fn == nil {
			err = fmt.Errorf("%s is not an exported function", funcName)
			return
		} else {
			wm.funcs[funcName] = fn
		}
	}

	mod = wm
	return
}

func (r *wasmtimeRuntime) Close(_ context.Context) error {
	r.engine = nil
	return nil // wasmtime only closes via finalizer
}

func (m *wasmtimeModule) CallI32_I32(_ context.Context, funcName string, param uint32) (uint32, error) {
	fn := m.funcs[funcName]
	if result, err := fn.Call(m.store, int32(param)); err != nil {
		return 0, err
	} else {
		return uint32(result.(int32)), nil
	}
}

func (m *wasmtimeModule) CallI32I32_V(_ context.Context, funcName string, x, y uint32) (err error) {
	fn := m.funcs[funcName]
	_, err = fn.Call(m.store, int32(x), int32(y))
	return
}

func (m *wasmtimeModule) CallI32_V(_ context.Context, funcName string, param uint32) (err error) {
	fn := m.funcs[funcName]
	_, err = fn.Call(m.store, int32(param))
	return
}

func (m *wasmtimeModule) CallI64_I64(_ context.Context, funcName string, param uint64) (uint64, error) {
	fn := m.funcs[funcName]
	if result, err := fn.Call(m.store, int64(param)); err != nil {
		return 0, err
	} else {
		return uint64(result.(int64)), nil
	}
}

func (m *wasmtimeModule) WriteMemory(_ context.Context, offset uint32, bytes []byte) error {
	unsafeSlice := m.mem.UnsafeData(m.store)
	copy(unsafeSlice[offset:], bytes)
	return nil
}

func (m *wasmtimeModule) Close(_ context.Context) error {
	m.store = nil
	m.instance = nil
	m.funcs = nil
	return nil // wasmtime only closes via finalizer
}
