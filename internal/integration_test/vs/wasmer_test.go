//go:build amd64 && cgo && !windows

package vs

import (
	"context"
	"fmt"

	"github.com/wasmerio/wasmer-go/wasmer"
)

func init() {
	runtimes["wasmer-go"] = newWasmerRuntime
}

func newWasmerRuntime() runtime {
	return &wasmerRuntime{}
}

type wasmerRuntime struct {
	engine *wasmer.Engine
}

type wasmerModule struct {
	store    *wasmer.Store
	module   *wasmer.Module
	instance *wasmer.Instance
	funcs    map[string]*wasmer.Function
}

func (r *wasmerRuntime) Compile(_ context.Context, _ *runtimeConfig) (err error) {
	r.engine = wasmer.NewEngine()
	// We can't reuse a store because even if we call close, re-instantiating too many times leads to:
	// >> resource limit exceeded: instance count too high at 10001
	return
}

func (r *wasmerRuntime) Instantiate(_ context.Context, cfg *runtimeConfig) (mod module, err error) {
	wm := &wasmerModule{funcs: map[string]*wasmer.Function{}}
	wm.store = wasmer.NewStore(r.engine)
	if wm.module, err = wasmer.NewModule(wm.store, cfg.moduleWasm); err != nil {
		return
	}
	importObject := wasmer.NewImportObject()

	// TODO: wasmer_module_set_name is not exposed in wasmer-go
	if wm.instance, err = wasmer.NewInstance(wm.module, importObject); err != nil {
		return
	}
	for _, funcName := range cfg.funcNames {
		var fn *wasmer.Function
		if fn, err = wm.instance.Exports.GetRawFunction(funcName); err != nil {
			return
		} else if fn == nil {
			return nil, fmt.Errorf("%s is not an exported function", funcName)
		} else {
			wm.funcs[funcName] = fn
		}
	}
	mod = wm
	return
}

func (r *wasmerRuntime) Close() error {
	r.engine = nil
	return nil
}

func (m *wasmerModule) CallI64_I64(_ context.Context, funcName string, param uint64) (uint64, error) {
	fn := m.funcs[funcName]
	if result, err := fn.Call(int64(param)); err != nil {
		return 0, err
	} else {
		return uint64(result.(int64)), nil
	}
}

func (m *wasmerModule) Close() error {
	if instance := m.instance; instance != nil {
		instance.Close()
	}
	m.instance = nil
	if mod := m.module; mod != nil {
		mod.Close()
	}
	m.module = nil
	if store := m.store; store != nil {
		store.Close()
	}
	m.store = nil
	m.funcs = nil
	return nil
}
