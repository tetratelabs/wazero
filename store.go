package wazero

import (
	"context"
	"fmt"

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
	// Context is the default context used to initialize the module. Defaults to context.Background.
	//
	// Notes:
	// * If the Module defines a start function, this is used to invoke it.
	// * This is the outer-most ancestor of wasm.ModuleContext Context() during wasm.HostFunction invocations.
	// * This is the default context of wasm.Function when callers pass nil.
	//
	// See https://www.w3.org/TR/wasm-core-1/#start-function%E2%91%A0
	Context context.Context
	// Engine defaults to NewEngineInterpreter
	Engine *Engine
}

func NewStore() wasm.Store {
	return internalwasm.NewStore(context.Background(), interpreter.NewEngine())
}

// NewStoreWithConfig returns a store with the given configuration.
func NewStoreWithConfig(config *StoreConfig) wasm.Store {
	ctx := config.Context
	if ctx == nil {
		ctx = context.Background()
	}
	engine := config.Engine
	if engine == nil {
		engine = NewEngineInterpreter()
	}
	return internalwasm.NewStore(ctx, engine.e)
}

// InstantiateModule instantiates the module namespace or errs if the configuration was invalid.
//
// Ex.
//	exports, _ := wazero.InstantiateModule(wazero.NewStore(), mod)
//
// Note: StoreConfig.Context is used for any WebAssembly 1.0 (MVP) Start Function.
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
