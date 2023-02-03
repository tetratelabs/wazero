//go:build cgo

package wasmtime

import (
	"context"
	"fmt"

	wt "github.com/bytecodealliance/wasmtime-go/v5"

	"github.com/tetratelabs/wazero/internal/integration_test/vs"
)

func newWasmtimeRuntime() vs.Runtime {
	return &wasmtimeRuntime{}
}

type wasmtimeRuntime struct {
	engine *wt.Engine
	module *wt.Module
}

type wasmtimeModule struct {
	store *wt.Store
	// instance is here because there's no close/destroy function. The only thing is garbage collection.
	instance *wt.Instance
	funcs    map[string]*wt.Func
	logFn    func([]byte) error
	mem      *wt.Memory
}

func (r *wasmtimeRuntime) Name() string {
	return "wasmtime"
}

func (m *wasmtimeModule) log(_ *wt.Caller, args []wt.Val) ([]wt.Val, *wt.Trap) {
	unsafeSlice := m.mem.UnsafeData(m.store)
	offset := args[0].I32()
	byteCount := args[1].I32()
	if err := m.logFn(unsafeSlice[offset : offset+byteCount]); err != nil {
		return nil, wt.NewTrap(err.Error())
	}
	return []wt.Val{}, nil
}

func (r *wasmtimeRuntime) Compile(_ context.Context, cfg *vs.RuntimeConfig) (err error) {
	r.engine = wt.NewEngine()
	if r.module, err = wt.NewModule(r.engine, cfg.ModuleWasm); err != nil {
		return
	}
	return
}

func (r *wasmtimeRuntime) Instantiate(_ context.Context, cfg *vs.RuntimeConfig) (mod vs.Module, err error) {
	wm := &wasmtimeModule{funcs: map[string]*wt.Func{}}

	// We can't reuse a store because even if we call close, re-instantiating too many times leads to:
	// >> resource limit exceeded: instance count too high at 10001
	wm.store = wt.NewStore(r.engine)
	linker := wt.NewLinker(wm.store.Engine)

	// Instantiate WASI, if configured.
	if cfg.NeedsWASI {
		if err = linker.DefineWasi(); err != nil {
			return
		}
		config := wt.NewWasiConfig() // defaults to toss stdout
		config.InheritStderr()       // see errors
		wm.store.SetWasi(config)
	}

	// Instantiate the host module, "env", if configured.
	if cfg.LogFn != nil {
		wm.logFn = cfg.LogFn
		if err = linker.Define("env", "log", wt.NewFunc(
			wm.store,
			wt.NewFuncType(
				[]*wt.ValType{
					wt.NewValType(wt.KindI32),
					wt.NewValType(wt.KindI32),
				},
				[]*wt.ValType{},
			),
			wm.log,
		)); err != nil {
			return
		}
	} else if cfg.EnvFReturnValue != 0 {
		ret := []wt.Val{wt.ValI64(int64(cfg.EnvFReturnValue))}
		if err = linker.Define("env", "f", wt.NewFunc(
			wm.store,
			wt.NewFuncType(
				[]*wt.ValType{
					wt.NewValType(wt.KindI64),
				},
				[]*wt.ValType{wt.NewValType(wt.KindI64)},
			),
			func(_ *wt.Caller, args []wt.Val) ([]wt.Val, *wt.Trap) {
				return ret, nil
			},
		)); err != nil {
			return
		}
	}

	// Set the module name.
	if err = linker.DefineModule(wm.store, cfg.ModuleName, r.module); err != nil {
		return
	}

	// Instantiate the module.
	instance, instantiateErr := linker.Instantiate(wm.store, r.module)
	if instantiateErr != nil {
		err = instantiateErr
		return
	}

	if cfg.LogFn != nil || cfg.NeedsMemoryExport {
		// Wasmtime does not allow a host function parameter for memory, so you have to manually propagate it.
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

func (r *wasmtimeRuntime) Close(context.Context) error {
	r.module = nil
	r.engine = nil
	return nil // wt only closes via finalizer
}

func (m *wasmtimeModule) Memory() []byte {
	return m.mem.UnsafeData(m.store)
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

func (m *wasmtimeModule) CallV_V(_ context.Context, funcName string) (err error) {
	fn := m.funcs[funcName]
	_, err = fn.Call(m.store)
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

func (m *wasmtimeModule) WriteMemory(offset uint32, bytes []byte) error {
	unsafeSlice := m.mem.UnsafeData(m.store)
	copy(unsafeSlice[offset:], bytes)
	return nil
}

func (m *wasmtimeModule) Close(context.Context) error {
	m.store = nil
	m.instance = nil
	m.funcs = nil
	return nil // wt only closes via finalizer
}
