//go:build amd64 && cgo && !windows

package wasm3

import (
	"context"
	"errors"
	"fmt"

	"github.com/birros/go-wasm3"

	"github.com/tetratelabs/wazero/internal/integration_test/vs"
)

func newWasm3Runtime() vs.Runtime {
	return &wasm3Runtime{}
}

type wasm3Runtime struct {
	runtime *wasm3.Runtime
}

type wasm3Module struct {
	module *wasm3.Module
	funcs  map[string]wasm3.FunctionWrapper
}

func (r *wasm3Runtime) Name() string {
	return "wasm3"
}

func (r *wasm3Runtime) Compile(_ context.Context, _ *vs.RuntimeConfig) (err error) {
	r.runtime = wasm3.NewRuntime(&wasm3.Config{
		Environment: wasm3.NewEnvironment(),
		StackSize:   64 * 1024, // from example
	})
	// There's currently no way to clone a parsed module, so we have to do it on instantiate.
	return
}

func (r *wasm3Runtime) Instantiate(_ context.Context, cfg *vs.RuntimeConfig) (mod vs.Module, err error) {
	m := &wasm3Module{funcs: map[string]wasm3.FunctionWrapper{}}

	m.module, err = r.runtime.ParseModule(cfg.ModuleWasm)

	// TODO: not sure we can set cfg.ModuleName in wasm3-go

	// Instantiate the module.
	if _, err = r.runtime.LoadModule(m.module); err != nil {
		return
	}

	// Ensure function exports exist.
	for _, funcName := range cfg.FuncNames {
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

func (r *wasm3Runtime) Close(_ context.Context) error {
	if r := r.runtime; r != nil {
		r.Destroy()
	}
	r.runtime = nil
	return nil
}

func (m *wasm3Module) CallI32_I32(_ context.Context, funcName string, param uint32) (uint32, error) {
	fn := m.funcs[funcName]
	// Note: go-wasm3 only maps the int type on input, though output params map based on value type
	if results, err := fn(int(param)); err != nil {
		return 0, err
	} else {
		return uint32(results[0].(int32)), nil
	}
}

func (m *wasm3Module) CallI32I32_V(_ context.Context, funcName string, x, y uint32) (err error) {
	fn := m.funcs[funcName]
	// Note: go-wasm3 only maps the int type on input, though output params map based on value type
	_, err = fn(int(x), int(y))
	return
}

func (m *wasm3Module) CallI32_V(_ context.Context, funcName string, param uint32) (err error) {
	fn := m.funcs[funcName]
	// Note: go-wasm3 only maps the int type on input, though output params map based on value type
	_, err = fn(int(param))
	return
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

func (m *wasm3Module) WriteMemory(_ context.Context, offset uint32, bytes []byte) error {
	return errors.New("TODO: https://github.com/birros/go-wasm3/issues/5")
}

func (m *wasm3Module) Close(_ context.Context) error {
	// module can't be destroyed
	m.module = nil
	m.funcs = nil
	return nil
}
