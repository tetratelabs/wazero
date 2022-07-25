package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/assemblyscript"
)

// asWasm compiled using `npm install && npm run build`
//
//go:embed testdata/assemblyscript.wasm
var asWasm []byte

// main shows how to interact with a WebAssembly function that was compiled
// from AssemblyScript
//
// See README.md for a full description.
func main() {

	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	// Use WebAssembly 2.0 because AssemblyScript uses some >1.0 features.
	r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfig().
		WithWasmCore2())
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Instantiate a module implementing functions used by AssemblyScript.
	// Thrown errors will be logged to os.Stderr
	_, err := assemblyscript.Instantiate(ctx, r)
	if err != nil {
		log.Panicln(err)
	}

	// Compile the WebAssembly module using the default configuration.
	code, err := r.CompileModule(ctx, asWasm, wazero.NewCompileConfig())
	if err != nil {
		log.Panicln(err)
	}

	// Instantiate a WebAssembly module that imports the "abort" and "trace"
	// functions defined by assemblyscript.Instantiate and exports functions
	// we'll use in this example.
	mod, err := r.InstantiateModule(ctx, code, wazero.NewModuleConfig().
		// Override the default module config that discards stdout and stderr.
		WithStdout(os.Stdout).WithStderr(os.Stderr))
	if err != nil {
		log.Panicln(err)
	}

	// Get references to WebAssembly functions we'll use in this example.
	helloWorld := mod.ExportedFunction("hello_world")
	goodbyeWorld := mod.ExportedFunction("goodbye_world")

	// Let's use the argument to this main function in Wasm.
	numStr := os.Args[1]
	num, err := strconv.Atoi(numStr)
	if err != nil {
		log.Panicln(err)
	}

	// Call hello_world, which returns the input value incremented by 3.
	// While this calls trace(), our configuration didn't enable it.
	results, err := helloWorld.Call(ctx, uint64(num))
	if err != nil {
		log.Panicln(err)
	}
	fmt.Printf("hello_world returned: %v", results[0])

	// Call goodbye_world, which aborts with an error.
	// assemblyscript.Instantiate was configured above to abort to stderr.
	results, err = goodbyeWorld.Call(ctx)
	if err == nil {
		log.Panicln("goodbye_world did not fail")
	}
}
