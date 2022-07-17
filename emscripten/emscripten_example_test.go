package emscripten_test

import (
	"context"
	_ "embed"
	"log"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/emscripten"
	"github.com/tetratelabs/wazero/wasi_snapshot_preview1"
)

// This shows how to instantiate Emscripten function imports.
func Example_instantiate() {
	ctx := context.Background()

	r := wazero.NewRuntime()
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Add WASI which is typically required when using Emscripten.
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		log.Panicln(err)
	}

	// Now, add the "env" module to the runtime, Emscripten default imports.
	if _, err := emscripten.Instantiate(ctx, r); err != nil {
		log.Panicln(err)
	}

	// Output:
}

// This shows how to instantiate Emscripten function imports when you also need
// other functions in the "env" module.
func Example_functionExporter() {
	ctx := context.Background()

	r := wazero.NewRuntime()
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Add WASI which is typically required when using Emscripten.
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		log.Panicln(err)
	}

	// Next, construct your own module builder for "env" with any functions
	// you need.
	envBuilder := r.NewModuleBuilder("env").
		ExportFunction("get_int", func() uint32 { return 1 })

	// Now, add Emscripten special function imports into it.
	emscripten.NewFunctionExporter().ExportFunctions(envBuilder)

	// Output:
}
