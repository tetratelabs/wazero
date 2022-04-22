//go:build amd64 && cgo

package vs

import (
	"context"
	"fmt"

	"github.com/bytecodealliance/wasmtime-go"
)

func init() {
	runtimeTesters["wasmtime-go"] = newWasmtimeTester
}

func newWasmtimeTester() runtimeTester {
	return &wasmtimeTester{funcs: map[string]*wasmtime.Func{}}
}

type wasmtimeTester struct {
	store *wasmtime.Store
	funcs map[string]*wasmtime.Func
}

func (w *wasmtimeTester) Init(_ context.Context, cfg *runtimeConfig) (err error) {
	w.store = wasmtime.NewStore(wasmtime.NewEngine())
	module, err := wasmtime.NewModule(w.store.Engine, cfg.moduleWasm)
	if err != nil {
		return
	}
	// TODO: not sure we can set cfg.moduleName in wasmtime-go
	instance, err := wasmtime.NewInstance(w.store, module, nil)
	if err != nil {
		return
	}
	for _, funcName := range cfg.funcNames {
		if fn := instance.GetFunc(w.store, funcName); fn == nil {
			return fmt.Errorf("%s is not an exported function", funcName)
		} else {
			w.funcs[funcName] = fn
		}
	}
	return
}

func (w *wasmtimeTester) CallI64_I64(_ context.Context, funcName string, param uint64) (uint64, error) {
	fn := w.funcs[funcName]
	if result, err := fn.Call(w.store, int64(param)); err != nil {
		return 0, err
	} else {
		return uint64(result.(int64)), nil
	}
}

func (w *wasmtimeTester) Close() error {
	return nil
}
