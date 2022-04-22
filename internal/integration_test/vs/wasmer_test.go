//go:build amd64 && cgo && !windows

package vs

import (
	"context"
	"fmt"

	"github.com/wasmerio/wasmer-go/wasmer"
)

func init() {
	runtimeTesters["wasmer-go"] = newWasmerTester
}

func newWasmerTester() runtimeTester {
	return &wasmerTester{funcs: map[string]*wasmer.Function{}}
}

type wasmerTester struct {
	store    *wasmer.Store
	module   *wasmer.Module
	instance *wasmer.Instance
	funcs    map[string]*wasmer.Function
}

func (w *wasmerTester) Init(_ context.Context, wasm []byte, funcNames ...string) (err error) {
	w.store = wasmer.NewStore(wasmer.NewEngine())
	importObject := wasmer.NewImportObject()
	if w.module, err = wasmer.NewModule(w.store, wasm); err != nil {
		return
	}
	if w.instance, err = wasmer.NewInstance(w.module, importObject); err != nil {
		return
	}
	for _, funcName := range funcNames {
		var fn *wasmer.Function
		if fn, err = w.instance.Exports.GetRawFunction(funcName); err != nil {
			return
		} else if fn == nil {
			return fmt.Errorf("%s is not an exported function", funcName)
		} else {
			w.funcs[funcName] = fn
		}
	}
	return
}

func (w *wasmerTester) CallI64_I64(_ context.Context, funcName string, param uint64) (uint64, error) {
	fn := w.funcs[funcName]
	if result, err := fn.Call(int64(param)); err != nil {
		return 0, err
	} else {
		return uint64(result.(int64)), nil
	}
}

func (w *wasmerTester) Close() error {
	for _, closer := range []func(){w.instance.Close, w.module.Close, w.store.Close} {
		if closer == nil {
			continue
		}
		closer()
	}
	w.instance = nil
	w.module = nil
	w.store = nil
	return nil
}
