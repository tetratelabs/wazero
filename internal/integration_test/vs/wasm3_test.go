//go:build amd64 && cgo && !windows

package vs

import (
	"context"
	"fmt"

	"github.com/birros/go-wasm3"
)

func init() {
	runtimeTesters["wasm3-go"] = newWasm3Tester
}

func newWasm3Tester() runtimeTester {
	return &wasm3Tester{funcs: map[string]wasm3.FunctionWrapper{}}
}

type wasm3Tester struct {
	runtime *wasm3.Runtime
	funcs   map[string]wasm3.FunctionWrapper
}

func (w *wasm3Tester) Init(_ context.Context, wasm []byte, funcNames ...string) (err error) {
	w.runtime = wasm3.NewRuntime(&wasm3.Config{
		Environment: wasm3.NewEnvironment(),
		StackSize:   64 * 1024, // from example
	})

	module, err := w.runtime.ParseModule(wasm)
	if err != nil {
		return err
	}

	_, err = w.runtime.LoadModule(module)
	if err != nil {
		return err
	}

	for _, funcName := range funcNames {
		var fn wasm3.FunctionWrapper
		if fn, err = w.runtime.FindFunction(funcName); err != nil {
			return
		} else if fn == nil {
			return fmt.Errorf("%s is not an exported function", funcName)
		} else {
			w.funcs[funcName] = fn
		}
	}
	return
}

func (w *wasm3Tester) CallI64_I64(_ context.Context, funcName string, param uint64) (uint64, error) {
	fn := w.funcs[funcName]
	// Note: go-wasm3 only maps the int type on input, though output params map based on value type
	if results, err := fn(int(param)); err != nil {
		return 0, err
	} else {
		return uint64(results[0].(int64)), nil
	}
}

func (w *wasm3Tester) Close() error {
	for _, closer := range []func(){w.runtime.Destroy} {
		if closer == nil {
			continue
		}
		closer()
	}
	w.runtime = nil
	return nil
}
