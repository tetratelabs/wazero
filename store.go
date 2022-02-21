package wazero

import (
	"fmt"

	"github.com/tetratelabs/wazero/internal/makefunc"
	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/interpreter"
	"github.com/tetratelabs/wazero/internal/wasm/jit"
	"github.com/tetratelabs/wazero/wasm"
)

type Engine struct {
	e internalwasm.Engine
}

func NewEngineInterpreter() *Engine {
	return &Engine{e: interpreter.NewEngine()}
}

func NewEngineJIT() *Engine { // TODO: compiler?
	return &Engine{e: jit.NewEngine()}
}

// StoreConfig allows customization of a Store via NewStoreWithConfig
type StoreConfig struct {
	// Engine defaults to NewEngineInterpreter
	Engine *Engine
}

func NewStore() wasm.Store {
	return internalwasm.NewStore(interpreter.NewEngine())
}

// NewStoreWithConfig returns a store with the given configuration.
func NewStoreWithConfig(config *StoreConfig) wasm.Store {
	engine := config.Engine
	if engine == nil {
		engine = NewEngineInterpreter()
	}
	return internalwasm.NewStore(engine.e)
}

// InstantiateModule instantiates the module namespace or errs if the configuration was invalid.
func InstantiateModule(store wasm.Store, module *Module) (wasm.ModuleExports, error) {
	internal, ok := store.(*internalwasm.Store)
	if !ok {
		return nil, fmt.Errorf("unsupported Store implementation: %s", store)
	}
	return internal.Instantiate(module.wasm, module.name)
}

// ExportHostFunctions adds functions written in Go, which a WebAssembly Module can import.
//
// Here is a description of parameters:
// * moduleName is the module name that these functions are imported with. Ex. wasi.ModuleSnapshotPreview1
// * nameToGoFunc map key is the name to export and the value is the func. Ex. WASISnapshotPreview1
//
// Noting a context exception described later, all parameters or result types must match WebAssembly 1.0 (MVP) value
// types. This means uint32, uint64, float32 or float64. Up to one result can be returned.
//
// Ex. This is a valid host function:
//
//	addInts := func(x uint32, uint32) uint32 {
//		return x + y
//	}
//
// Host functions may also have an initial parameter (param[0]) of type context.Context or wasm.ModuleContext.
//
// Ex. This uses a Go Context:
//
//	addInts := func(ctx context.Context, x uint32, uint32) uint32 {
//		// add a little extra if we put some in the context!
//		return x + y + ctx.Value(extraKey).(uint32)
//	}
//
// The most sophisticated context is wasm.ModuleContext, which allows access to the Go context, but also
// allows writing to memory. This is important because there are only numeric types in Wasm. The only way to share other
// data is via writing memory and sharing offsets.
//
// Ex. This reads the parameters from!
//
//	addInts := func(ctx wasm.ModuleContext, offset uint32) uint32 {
//		x, _ := ctx.Memory().ReadUint32Le(offset)
//		y, _ := ctx.Memory().ReadUint32Le(offset + 4) // 32 bits == 4 bytes!
//		return x + y
//	}
//
// See https://www.w3.org/TR/wasm-core-1/#host-functions%E2%91%A2
func ExportHostFunctions(store wasm.Store, moduleName string, nameToGoFunc map[string]interface{}) (wasm.HostExports, error) {
	internal, ok := store.(*internalwasm.Store)
	if !ok {
		return nil, fmt.Errorf("unsupported Store implementation: %s", store)
	}
	return internal.ExportHostFunctions(moduleName, nameToGoFunc)
}

// MakeWasmFunc implements the goFuncPtr so that it calls an exported function or errs if the function isn't in the
// module or there's a signature mismatch.
//
// * functions is the module's exported functions
// * name is the exported function name in the current Wasm module
// * goFuncPtr is a pointer to a func and will have its value replaced on success
//
// Ex. Given the below the export and type use of the function in WebAssembly 1.0 (MVP) Text Format:
//	(func (export "AddInt") (param i32 i32) (result i32) ...
//
// The following would bind that to the Go func named addInt:
//
//	var addInt func(uint32, uint32) uint32
//	err = MakeWasmFunc(exports, "AddInt", &addInt)
//	ret, err := addInt(1, 2) // ret == 3, err == nil
//
// Notes on func signature:
// * Parameters may begin with a context.Context and if not defaults to context.Background.
// * Results may end with an error, and if not, any error calling the function will panic.
// * Otherwise, parameters and results must map to WebAssembly 1.0 (MVP) Value types.
func MakeWasmFunc(exports wasm.ModuleExports, name string, goFuncPtr interface{}) (err error) {
	internal, ok := exports.(*internalwasm.ModuleContext)
	if !ok {
		return fmt.Errorf("unsupported ModuleExports implementation: %s", exports)
	}
	exp, err := internal.Module.GetExport(name, internalwasm.ExportKindFunc)
	if err != nil {
		return err
	}

	// TODO: consider how caching could work. Be careful to not explode cardinality as the goFuncPtr would change in
	// each host function call, and the signature can also be different (ex with and w/o context, with and w/o error).
	return makefunc.MakeWasmFunc(internal, name, exp.Function, goFuncPtr)
}

func MakeHostFunc(exports wasm.HostExports, name string, goFuncPtr interface{}) (err error) {
	internal, ok := exports.(*internalwasm.HostExports)
	if !ok {
		return fmt.Errorf("unsupported HostExports implementation: %s", exports)
	}
	fn, ok := internal.NameToFunctionInstance[name]
	if !ok {
		return fmt.Errorf("%s is not a HostFunction", name)
	}
	return makefunc.MakeWasmFunc(nil, name, fn, goFuncPtr)
}
