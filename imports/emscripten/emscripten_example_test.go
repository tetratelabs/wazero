package emscripten_test

import (
	"context"
	_ "embed"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/emscripten"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// This shows how to instantiate Emscripten function imports.
func Example_instantiate() {
	ctx := context.Background()

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Add WASI which is typically required when using Emscripten.
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	// Now, add the "env" module to the runtime, Emscripten default imports.
	emscripten.MustInstantiate(ctx, r)

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

	// Next, construct your own module builder for "env" with any functions
	// you need.
	envBuilder := r.NewHostModuleBuilder("env").
		NewFunctionBuilder().
		WithFunc(func() uint32 { return 1 }).
		Export("get_int")

	// Now, add Emscripten special function imports into it.
	emscripten.NewFunctionExporter().ExportFunctions(envBuilder)

	// Output:
}
