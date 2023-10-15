package emscripten_test

import (
	"context"
	_ "embed"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/emscripten"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed testdata/invoke.wasm
var invokeWasm []byte

// This shows how to instantiate Emscripten function imports.
func Example_instantiateForModule() {
	ctx := context.Background()

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Add WASI which is typically required when using Emscripten.
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	// Compile the WASM so wazero can handle dynamically generated imports.
	compiled, err := r.CompileModule(ctx, invokeWasm)
	if err != nil {
		panic(err)
	}

	envCloser, err := emscripten.InstantiateForModule(ctx, r, compiled)
	if err != nil {
		panic(err)
	}
	defer envCloser.Close(ctx) // This closes the env module.

	// Output:
}

// This shows how to instantiate Emscripten function imports when you also need
// other functions in the "env" module.
func Example_functionExporter() {
	ctx := context.Background()

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Add WASI which is typically required when using Emscripten.
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	// Compile the WASM so wazero can handle dynamically generated imports.
	compiled, err := r.CompileModule(ctx, invokeWasm)
	if err != nil {
		panic(err)
	}
	exporter, err := emscripten.NewFunctionExporterForModule(compiled)
	if err != nil {
		panic(err)
	}
	// Next, construct your own module builder for "env" with any functions
	// you need.
	envBuilder := r.NewHostModuleBuilder("env").
		NewFunctionBuilder().
		WithFunc(func() uint32 { return 1 }).
		Export("get_int")

	// Now, add Emscripten special function imports into it.
	exporter.ExportFunctions(envBuilder)
	// Output:
}
