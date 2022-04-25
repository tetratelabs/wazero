//go:build amd64 && cgo

package vs

import (
	"context"
	"fmt"

	"github.com/bytecodealliance/wasmtime-go"
)

func init() {
	runtimes["wasmtime-go"] = newWasmtimeRuntime
}

func newWasmtimeRuntime() runtime {
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
}

func (r *wasmtimeRuntime) Compile(_ context.Context, _ *runtimeConfig) (err error) {
	r.engine = wasmtime.NewEngine()
	// We can't reuse a store because even if we call close, re-instantiating too many times leads to:
	// >> resource limit exceeded: instance count too high at 10001
	return
}

func (r *wasmtimeRuntime) Instantiate(_ context.Context, cfg *runtimeConfig) (mod module, err error) {
	wm := &wasmtimeModule{funcs: map[string]*wasmtime.Func{}}
	wm.store = wasmtime.NewStore(r.engine)
	var m *wasmtime.Module
	if m, err = wasmtime.NewModule(wm.store.Engine, cfg.moduleWasm); err != nil {
		return
	}

	// Set the module name
	linker := wasmtime.NewLinker(wm.store.Engine)
	if err = linker.DefineModule(wm.store, cfg.moduleName, m); err != nil {
		return
	}
	instance, err := linker.Instantiate(wm.store, m)
	if err != nil {
		return
	}

	for _, funcName := range cfg.funcNames {
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

func (m *wasmtimeModule) CallI64_I64(_ context.Context, funcName string, param uint64) (uint64, error) {
	fn := m.funcs[funcName]
	if result, err := fn.Call(m.store, int64(param)); err != nil {
		return 0, err
	} else {
		return uint64(result.(int64)), nil
	}
}

func (m *wasmtimeModule) Close(_ context.Context) error {
	m.store = nil
	m.instance = nil
	m.funcs = nil
	return nil // wasmtime only closes via finalizer
}
