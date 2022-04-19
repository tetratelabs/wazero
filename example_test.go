package wazero

import (
	"context"
	_ "embed"
	"fmt"
	"log"
)

// This is an example of how to use WebAssembly via adding two numbers.
//
// See https://github.com/tetratelabs/wazero/tree/main/examples for more examples.
func Example() {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := NewRuntime()

	// Add a module to the runtime named "wasm/math" which exports one function "add", implemented in WebAssembly.
	mod, err := r.InstantiateModuleFromCode(ctx, []byte(`(module $wasm/math
    (func $add (param i32 i32) (result i32)
        local.get 0
        local.get 1
        i32.add
    )
    (export "add" (func $add))
)`))
	if err != nil {
		log.Fatal(err)
	}
	defer mod.Close()

	// Get a function that can be reused until its module is closed:
	add := mod.ExportedFunction("add")

	x, y := uint64(1), uint64(2)
	results, err := add.Call(ctx, x, y)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%s: %d + %d = %d\n", mod.Name(), x, y, results[0])

	// Output:
	// wasm/math: 1 + 2 = 3
}
