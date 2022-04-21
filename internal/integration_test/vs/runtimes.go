package vs

import "C"
import (
	"context"
	"fmt"
	"io"

	"github.com/birros/go-wasm3"
	"github.com/bytecodealliance/wasmtime-go"
	"github.com/wasmerio/wasmer-go/wasmer"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/wasi"
)

type runtimeTester interface {
	Init(ctx context.Context, wasm []byte, funcNames ...string) error
	Call(ctx context.Context, funcName string, params ...uint64) (uint64, error)
	io.Closer
}

func newWazeroTester(config *wazero.RuntimeConfig) runtimeTester {
	return &wazeroTester{config: config, funcs: map[string]api.Function{}}
}

type wazeroTester struct {
	config    *wazero.RuntimeConfig
	wasi, mod api.Module
	funcs     map[string]api.Function
}

func (w *wazeroTester) Init(ctx context.Context, wasm []byte, funcNames ...string) (err error) {
	r := wazero.NewRuntimeWithConfig(w.config)

	if w.wasi, err = wasi.InstantiateSnapshotPreview1(ctx, r); err != nil {
		return
	}
	if w.mod, err = r.InstantiateModuleFromCode(ctx, wasm); err != nil {
		return
	}
	for _, funcName := range funcNames {
		if fn := w.mod.ExportedFunction(funcName); fn == nil {
			return fmt.Errorf("%s is not an exported function", fn)
		} else {
			w.funcs[funcName] = fn
		}
	}
	return
}

func (w *wazeroTester) Call(ctx context.Context, funcName string, params ...uint64) (uint64, error) {
	if results, err := w.funcs[funcName].Call(ctx, params...); err != nil {
		return 0, err
	} else if len(results) > 0 {
		return results[0], nil
	}
	return 0, nil
}

func (w *wazeroTester) Close() (err error) {
	for _, closer := range []io.Closer{w.mod, w.wasi} {
		if closer == nil {
			continue
		}
		if nextErr := closer.Close(); nextErr != nil {
			err = nextErr
		}
	}
	w.mod = nil
	w.wasi = nil
	return
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

func (w *wasmerTester) Call(_ context.Context, funcName string, params ...uint64) (uint64, error) {
	fn := w.funcs[funcName]
	iParams := make([]interface{}, len(params))
	for i := range params {
		switch fn.Type().Params()[i].Kind() {
		case wasmer.I32:
			iParams[i] = int32(params[i])
		case wasmer.I64:
			iParams[i] = int64(params[i])
		}
	}
	if result, err := fn.Call(iParams...); err != nil {
		return 0, err
	} else if fn.ResultArity() == 1 {
		if i, ok := result.(int32); ok {
			return uint64(i), nil
		}
		if i, ok := result.(int64); ok {
			return uint64(i), nil
		}
	}
	return 0, nil
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

func newWasmtimeTester() runtimeTester {
	return &wasmtimeTester{funcs: map[string]*wasmtime.Func{}}
}

type wasmtimeTester struct {
	store *wasmtime.Store
	funcs map[string]*wasmtime.Func
}

func (w *wasmtimeTester) Init(_ context.Context, wasm []byte, funcNames ...string) (err error) {
	w.store = wasmtime.NewStore(wasmtime.NewEngine())
	module, err := wasmtime.NewModule(w.store.Engine, wasm)
	if err != nil {
		return
	}
	instance, err := wasmtime.NewInstance(w.store, module, nil)
	if err != nil {
		return
	}
	for _, funcName := range funcNames {
		if fn := instance.GetFunc(w.store, funcName); fn == nil {
			return fmt.Errorf("%s is not an exported function", funcName)
		} else {
			w.funcs[funcName] = fn
		}
	}
	return
}

func (w *wasmtimeTester) Call(_ context.Context, funcName string, params ...uint64) (uint64, error) {
	fn := w.funcs[funcName]
	iParams := make([]interface{}, len(params))
	for i := range params {
		switch fn.Type(w.store).Params()[i].Kind() {
		case wasmtime.KindI32:
			iParams[i] = int32(params[i])
		case wasmtime.KindI64:
			iParams[i] = int64(params[i])
		}
		iParams[i] = int(params[i])
	}
	if result, err := fn.Call(w.store, iParams...); err != nil {
		return 0, err
	} else if result != nil {
		if i, ok := result.(int32); ok {
			return uint64(i), nil
		}
		if i, ok := result.(int64); ok {
			return uint64(i), nil
		}
	}
	return 0, nil
}

func (w *wasmtimeTester) Close() error {
	return nil
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

func (w *wasm3Tester) Call(_ context.Context, funcName string, params ...uint64) (uint64, error) {
	fn := w.funcs[funcName]
	iParams := make([]interface{}, len(params))
	for i := range params {
		iParams[i] = int(params[i]) // go-wasm3 only maps the int type
	}
	if results, err := fn(iParams...); err != nil {
		return 0, err
	} else if len(results) == 1 {
		if i, ok := results[0].(int32); ok {
			return uint64(i), nil
		}
		if i, ok := results[0].(int64); ok {
			return uint64(i), nil
		}
	}
	return 0, nil
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
