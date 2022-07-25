package assemblyscript_test

import (
	"context"
	_ "embed"
	"log"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/assemblyscript"
)

// This shows how to instantiate AssemblyScript's special imports.
func Example_instantiate() {
	ctx := context.Background()

	r := wazero.NewRuntime()
	defer r.Close(ctx) // This closes everything this Runtime created.

	// This adds the "env" module to the runtime, with AssemblyScript's special
	// function imports.
	if _, err := assemblyscript.Instantiate(ctx, r); err != nil {
		log.Panicln(err)
	}

	// Output:
}

// This shows how to instantiate AssemblyScript's special imports when you also
// need other functions in the "env" module.
func Example_functionExporter() {
	ctx := context.Background()

	r := wazero.NewRuntime()
	defer r.Close(ctx) // This closes everything this Runtime created.

	// First construct your own module builder for "env"
	envBuilder := r.NewModuleBuilder("env").
		ExportFunction("get_int", func() uint32 { return 1 })

	// Now, add AssemblyScript special function imports into it.
	assemblyscript.NewFunctionExporter().
		WithAbortMessageDisabled().
		ExportFunctions(envBuilder)

	// Output:
}
