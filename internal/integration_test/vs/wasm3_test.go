//go:build amd64 && cgo && !windows

package vs

import (
	"context"
	"fmt"

	"github.com/birros/go-wasm3"
)

func init() {
	runtimes["wasm3-go"] = newWasm3Runtime
}

func newWasm3Runtime() runtime {
	return &wasm3Runtime{}
}

type wasm3Runtime struct {
	runtime *wasm3.Runtime
}
type wasm3Module struct {
	module *wasm3.Module
	funcs  map[string]wasm3.FunctionWrapper
}

func (r *wasm3Runtime) Compile(_ context.Context, _ *runtimeConfig) (err error) {
	r.runtime = wasm3.NewRuntime(&wasm3.Config{
		Environment: wasm3.NewEnvironment(),
		StackSize:   64 * 1024, // from example
	})
	// There's currently no way to clone a parsed module, so we have to do it on instantiate.
	return
}

func (r *wasm3Runtime) Instantiate(_ context.Context, cfg *runtimeConfig) (mod module, err error) {
	m := &wasm3Module{funcs: map[string]wasm3.FunctionWrapper{}}

	m.module, err = r.runtime.ParseModule(cfg.moduleWasm)

	// TODO: not sure we can set cfg.moduleName in wasm3-go
	if _, err = r.runtime.LoadModule(m.module); err != nil {
		return
	}

	for _, funcName := range cfg.funcNames {
		var fn wasm3.FunctionWrapper
		if fn, err = r.runtime.FindFunction(funcName); err != nil {
			return
		} else if fn == nil {
			err = fmt.Errorf("%s is not an exported function", funcName)
			return
		} else {
			m.funcs[funcName] = fn
		}
	}
	mod = m
	return
}

func (r *wasm3Runtime) Close() error {
	if r := r.runtime; r != nil {
		r.Destroy()
	}
	r.runtime = nil
	return nil
}

func (m *wasm3Module) CallI64_I64(_ context.Context, funcName string, param uint64) (uint64, error) {
	fn := m.funcs[funcName]
	// Note: go-wasm3 only maps the int type on input, though output params map based on value type
	if results, err := fn(int(param)); err != nil {
		return 0, err
	} else {
		return uint64(results[0].(int64)), nil
	}
}

func (m *wasm3Module) Close() error {
	// module can't be destroyed
	m.module = nil
	m.funcs = nil
	return nil
}
