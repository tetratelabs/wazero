package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// helloWasm was compiled using `cargo build --release --target wasm32-unknown-unknown`
//go:embed testdata/hello.wasm
var helloWasm []byte

// main shows how to interact with a WebAssembly function that was compiled
// from Rust.
//
// See README.md for a full description.
func main() {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime()

	// Instantiate a module named "env" that exports a function to log a string
	// to the console.
	env, err := r.NewModuleBuilder("env").
		ExportFunction("log", logString).
		Instantiate(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer env.Close()

	// Instantiate a module named "hello" that imports the "log" function
	// defined in "env".
	hello, err := r.InstantiateModuleFromCodeWithConfig(ctx, helloWasm, wazero.NewModuleConfig().WithName("hello"))
	if err != nil {
		log.Fatal(err)
	}
	defer hello.Close()

	// Get a references to functions we'll use in this example.
	sayHello := hello.ExportedFunction("say_hello")
	allocate := hello.ExportedFunction("allocate")
	deallocate := hello.ExportedFunction("deallocate")

	// Let's use the argument to this main function in Wasm.
	name := os.Args[1]
	byteCount := uint64(len(name))

	// Instead of an arbitrary memory offset, use Rust's allocator. Notice
	// there is nothing string-specific in this allocation function. The same
	// function could be used to pass serialized data like JSON to Wasm.
	results, err := allocate.Call(ctx, byteCount)
	if err != nil {
		log.Fatal(err)
	}
	pointer := results[0]
	// This pointer is managed by Rust, but Rust is unaware of external usage.
	// So, we have to deallocate it when finished
	defer deallocate.Call(ctx, pointer, byteCount)

	// The pointer is a linear memory offset, which is where we write the name.
	if !hello.Memory().Write(uint32(pointer), []byte(name)) {
		log.Fatalf("Memory.Write(%d, %d) out of range of memory size %d",
			pointer, byteCount, hello.Memory().Size())
	}

	// Now, we can call "say_hello", which reads the string we wrote to memory!
	_, err = sayHello.Call(ctx, pointer, byteCount)
	if err != nil {
		log.Fatal(err)
	}
}

func logString(m api.Module, offset, byteCount uint32) {
	buf, ok := m.Memory().Read(offset, byteCount)
	if !ok {
		log.Fatalf("Memory.Read(%d, %d) out of range", offset, byteCount)
	}
	fmt.Println(string(buf))
}
