//go:build amd64 && cgo && !windows

package wasmer

import (
	"context"
	"fmt"

	"github.com/wasmerio/wasmer-go/wasmer"

	"github.com/tetratelabs/wazero/internal/integration_test/vs"
)

var wasiEnv *wasmer.WasiEnvironment

func init() {
	var err error
	if wasiEnv, err = wasmer.NewWasiStateBuilder("").Finalize(); err != nil {
		panic(err)
	}
}

func newWasmerRuntime() vs.Runtime {
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
	logFn    func([]byte) error
	mem      *wasmer.Memory
}

func (r *wasmerRuntime) Name() string {
	return "wasmer"
}

func (m *wasmerModule) log(args []wasmer.Value) ([]wasmer.Value, error) {
	unsafeSlice := m.mem.Data()
	offset := args[0].I32()
	byteCount := args[1].I32()
	if err := m.logFn(unsafeSlice[offset : offset+byteCount]); err != nil {
		return nil, err
	}
	return []wasmer.Value{}, nil
}

func (r *wasmerRuntime) Compile(_ context.Context, _ *vs.RuntimeConfig) (err error) {
	r.engine = wasmer.NewEngine()
	// We can't reuse a store because even if we call close, re-instantiating too many times leads to:
	// >> resource limit exceeded: instance count too high at 10001
	return
}

func (r *wasmerRuntime) Instantiate(_ context.Context, cfg *vs.RuntimeConfig) (mod vs.Module, err error) {
	wm := &wasmerModule{funcs: map[string]*wasmer.Function{}}
	wm.store = wasmer.NewStore(r.engine)

	if wm.module, err = wasmer.NewModule(wm.store, cfg.ModuleWasm); err != nil {
		return
	}

	// Instantiate WASI, if configured.
	var importObject *wasmer.ImportObject
	if cfg.NeedsWASI {
		if importObject, err = wasiEnv.GenerateImportObject(wm.store, wm.module); err != nil {
			return
		}
	} else {
		importObject = wasmer.NewImportObject()
	}

	// Instantiate the host module, "env", if configured.
	if cfg.LogFn != nil {
		wm.logFn = cfg.LogFn
		importObject.Register(
			"env",
			map[string]wasmer.IntoExtern{
				"log": wasmer.NewFunction(
					wm.store,
					wasmer.NewFunctionType(wasmer.NewValueTypes(wasmer.I32, wasmer.I32), wasmer.NewValueTypes()),
					wm.log,
				),
			},
		)
	}

	// TODO: wasmer_module_set_name is not exposed in wasmer-go

	// Instantiate the module.
	if wm.instance, err = wasmer.NewInstance(wm.module, importObject); err != nil {
		return
	}

	// Wasmer does not allow a host function parameter for memory, so you have to manually propagate it.
	if cfg.LogFn != nil {
		if wm.mem, err = wm.instance.Exports.GetMemory("memory"); err != nil {
			return
		}
	}

	// If WASI is needed, we have to go back and invoke the _start function.
	if cfg.NeedsWASI {
		if start, startErr := wm.instance.Exports.GetWasiStartFunction(); startErr != nil {
			err = startErr
			return
		} else if _, err = start(); err != nil {
			return
		}
	}

	// Ensure function exports exist.
	for _, funcName := range cfg.FuncNames {
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

func (r *wasmerRuntime) Close(_ context.Context) error {
	r.engine = nil
	return nil
}

func (m *wasmerModule) CallI32_I32(_ context.Context, funcName string, param uint32) (uint32, error) {
	fn := m.funcs[funcName]
	if result, err := fn.Call(int32(param)); err != nil {
		return 0, err
	} else {
		return uint32(result.(int32)), nil
	}
}

func (m *wasmerModule) CallI32I32_V(_ context.Context, funcName string, x, y uint32) (err error) {
	fn := m.funcs[funcName]
	_, err = fn.Call(int32(x), int32(y))
	return
}

func (m *wasmerModule) CallI32_V(_ context.Context, funcName string, param uint32) (err error) {
	fn := m.funcs[funcName]
	_, err = fn.Call(int32(param))
	return
}

func (m *wasmerModule) CallI64_I64(_ context.Context, funcName string, param uint64) (uint64, error) {
	fn := m.funcs[funcName]
	if result, err := fn.Call(int64(param)); err != nil {
		return 0, err
	} else {
		return uint64(result.(int64)), nil
	}
}

func (m *wasmerModule) WriteMemory(_ context.Context, offset uint32, bytes []byte) error {
	unsafeSlice := m.mem.Data()
	copy(unsafeSlice[offset:], bytes)
	return nil
}

func (m *wasmerModule) Close(_ context.Context) error {
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
