package add

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// main implements a basic function in both Go and WebAssembly.
func main() {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime()

	// Add a module to the runtime named "wasm/math" which exports one function "add", implemented in WebAssembly.
	wasm, err := r.InstantiateModuleFromCode(ctx, []byte(`(module $wasm/math
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
	defer wasm.Close(ctx)

	// Add a module to the runtime named "host/math" which exports one function "add", implemented in Go.
	host, err := r.NewModuleBuilder("host/math").
		ExportFunction("add", func(v1, v2 uint32) uint32 {
			return v1 + v2
		}).Instantiate(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer host.Close(ctx)

	// Read two args to add.
	x, y := readTwoArgs()

	// Call the same function in both modules and print the results to the console.
	for _, mod := range []api.Module{wasm, host} {
		add := mod.ExportedFunction("add")
		results, err := add.Call(ctx, x, y)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("%s: %d + %d = %d\n", mod.Name(), x, y, results[0])
	}
}

func readTwoArgs() (uint64, uint64) {
	x, err := strconv.ParseUint(os.Args[1], 10, 64)
	if err != nil {
		log.Fatalf("invalid arg %v: %v", os.Args[1], err)
	}

	y, err := strconv.ParseUint(os.Args[2], 10, 64)
	if err != nil {
		log.Fatalf("invalid arg %v: %v", os.Args[2], err)
	}
	return x, y
}
