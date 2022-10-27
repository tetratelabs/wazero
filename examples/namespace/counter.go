package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// counterWasm was generated by the following:
//
//	cd testdata; wat2wasm --debug-names counter.wat
//
//go:embed testdata/counter.wasm
var counterWasm []byte

// main shows how to instantiate the same module name multiple times in the same runtime.
//
// See README.md for a full description.
func main() {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Compile WebAssembly that requires its own "env" module.
	compiled, err := r.CompileModule(ctx, counterWasm)
	if err != nil {
		log.Panicln(err)
	}

	// Instantiate two modules, with identical configuration, but independent state.
	m1 := instantiateWithEnv(ctx, r, compiled)
	m2 := instantiateWithEnv(ctx, r, compiled)

	for i := 0; i < 2; i++ {
		fmt.Printf("m1 counter=%d\n", counterGet(ctx, m1))
		fmt.Printf("m2 counter=%d\n", counterGet(ctx, m2))
	}
}

// count calls "counter.get" in the given namespace
func counterGet(ctx context.Context, mod api.Module) uint64 {
	results, err := mod.ExportedFunction("get").Call(ctx)
	if err != nil {
		log.Panicln(err)
	}
	return results[0]
}

// counter is an example showing state that needs to be independent per importing module.
type counter struct {
	counter uint32
}

func (e *counter) getAndIncrement(context.Context) (ret uint32) {
	ret = e.counter
	e.counter++
	return
}

// instantiateWithEnv returns a module instantiated with its own "env" module.
func instantiateWithEnv(ctx context.Context, r wazero.Runtime, module wazero.CompiledModule) api.Module {
	// Create a new namespace to instantiate modules into.
	ns := r.NewNamespace(ctx) // Note: this is closed when the Runtime is

	// Instantiate a new "env" module which exports a stateful function.
	c := &counter{}
	_, err := r.NewHostModuleBuilder("env").
		NewFunctionBuilder().WithFunc(c.getAndIncrement).Export("next_i32").
		Instantiate(ctx, ns)
	if err != nil {
		log.Panicln(err)
	}

	// Instantiate the module that imports "env".
	mod, err := ns.InstantiateModule(ctx, module, wazero.NewModuleConfig())
	if err != nil {
		log.Panicln(err)
	}

	return mod
}
