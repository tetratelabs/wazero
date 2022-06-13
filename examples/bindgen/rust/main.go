package main

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/bindgen"
)

// Generates the WASM from Rust
//go:generate cargo build --manifest-path testdata/Cargo.toml --target wasm32-unknown-unknown --release

// Embed the WASM
//go:embed target/wasm32-unknown-unknown/release/testdata.wasm
var wasm []byte

func main() {
	// Create the Wazero runtime
	ctx := context.Background()
	r := wazero.NewRuntime()
	defer r.Close(ctx)

	// Instantiate the bindgen module in the runtime before instantiating our WASM module.
	// This will load in some host functions that are required.
	bg, err := bindgen.Instantiate(ctx, r)
	if err != nil {
		panic(err)
	}

	// Instantiate it in the runtime
	module, err := r.InstantiateModuleFromBinary(ctx, wasm)
	if err != nil {
		panic(err)
	}
	bg.Bind(module)

	results, err := bg.Execute(ctx, "greet", "Wazero")
	if err != nil {
		panic(err)
	}
	fmt.Println(results[0].(string)) // Prints "Hello Wazero"

	_, err = bg.Execute(ctx, "greet_err", "Wazero")
	fmt.Println(err) // Prints "oops, there was an error"
}
