package wazero

import (
	"context"
	_ "embed"
	"fmt"
	"log"
)

// addWasm was generated by the following:
//
//	cd examples/basic/testdata/testdata; wat2wasm --debug-names add.wat
//
//go:embed examples/basic/testdata/add.wasm
var addWasm []byte

// This is an example of how to use WebAssembly via adding two numbers.
//
// See https://github.com/tetratelabs/wazero/tree/main/examples for more.
func Example() {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := NewRuntime()

	// Add a module to the runtime named "wasm/math" which exports one function
	// "add", implemented in WebAssembly.
	mod, err := r.InstantiateModuleFromBinary(ctx, addWasm)
	if err != nil {
		log.Panicln(err)
	}
	defer mod.Close(ctx)

	// Get a function that can be reused until its module is closed:
	add := mod.ExportedFunction("add")

	x, y := uint64(1), uint64(2)
	results, err := add.Call(ctx, x, y)
	if err != nil {
		log.Panicln(err)
	}

	fmt.Printf("%s: %d + %d = %d\n", mod.Name(), x, y, results[0])

	// Output:
	// wasm/math: 1 + 2 = 3
}
