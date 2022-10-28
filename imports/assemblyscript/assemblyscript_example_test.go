package assemblyscript_test

import (
	"context"
	_ "embed"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/assemblyscript"
)

// This shows how to instantiate AssemblyScript's special imports.
func Example_instantiate() {
	ctx := context.Background()

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.

	// This adds the "env" module to the runtime, with AssemblyScript's special
	// function imports.
	assemblyscript.MustInstantiate(ctx, r)

	// Output:
}

// This shows how to instantiate AssemblyScript's special imports when you also
// need other functions in the "env" module.
func Example_functionExporter() {
	ctx := context.Background()

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.

	// First construct your own module builder for "env"
	envBuilder := r.NewHostModuleBuilder("env").
		NewFunctionBuilder().
		WithFunc(func() uint32 { return 1 }).
		Export("get_int")

	// Now, add AssemblyScript special function imports into it.
	assemblyscript.NewFunctionExporter().
		WithAbortMessageDisabled().
		ExportFunctions(envBuilder)

	// Output:
}
