package vs

import (
	"context"
	"fmt"
	"io"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

type runtimeConfig struct {
	moduleName string
	moduleWasm []byte
	funcNames  []string
}

type runtime interface {
	Compile(ctx context.Context, cfg *runtimeConfig) error
	Instantiate(ctx context.Context, cfg *runtimeConfig) (module, error)
	io.Closer
}

type module interface {
	CallI64_I64(ctx context.Context, funcName string, param uint64) (uint64, error)
	io.Closer
}

func newWazeroInterpreterRuntime() runtime {
	return newWazeroRuntime(wazero.NewRuntimeConfigInterpreter().WithFinishedFeatures())
}

func newWazeroJITRuntime() runtime {
	return newWazeroRuntime(wazero.NewRuntimeConfigJIT().WithFinishedFeatures())
}

func newWazeroRuntime(config *wazero.RuntimeConfig) runtime {
	return &wazeroRuntime{config: config}
}

type wazeroRuntime struct {
	config   *wazero.RuntimeConfig
	runtime  wazero.Runtime
	compiled *wazero.CompiledCode
}

type wazeroModule struct {
	mod   api.Module
	funcs map[string]api.Function
}

func (r *wazeroRuntime) Compile(ctx context.Context, cfg *runtimeConfig) (err error) {
	r.runtime = wazero.NewRuntimeWithConfig(r.config)
	r.compiled, err = r.runtime.CompileModule(ctx, cfg.moduleWasm)
	return
}

func (r *wazeroRuntime) Instantiate(ctx context.Context, cfg *runtimeConfig) (mod module, err error) {
	wazeroCfg := wazero.NewModuleConfig().WithName(cfg.moduleName)
	m := &wazeroModule{funcs: map[string]api.Function{}}
	if m.mod, err = r.runtime.InstantiateModuleWithConfig(ctx, r.compiled, wazeroCfg); err != nil {
		return
	}
	for _, funcName := range cfg.funcNames {
		if fn := m.mod.ExportedFunction(funcName); fn == nil {
			return nil, fmt.Errorf("%s is not an exported function", fn)
		} else {
			m.funcs[funcName] = fn
		}
	}
	mod = m
	return
}

func (r *wazeroRuntime) Close() (err error) {
	if compiled := r.compiled; compiled != nil {
		err = compiled.Close()
	}
	r.compiled = nil
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

func (m *wazeroModule) Close() (err error) {
	if mod := m.mod; mod != nil {
		err = mod.Close()
	}
	m.mod = nil
	return
}
