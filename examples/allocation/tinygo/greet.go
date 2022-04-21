package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/wasi"
)

// greetWasm was compiled using `tinygo build -o greet.wasm -scheduler=none -target=wasi greet.go`
//go:embed testdata/greet.wasm
var greetWasm []byte

// main shows how to interact with a WebAssembly function that was compiled
// from TinyGo.
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

	// Note: testdata/greet.go doesn't use WASI, but TinyGo needs it to
	// implement functions such as panic.
	wm, err := wasi.InstantiateSnapshotPreview1(ctx, r)
	if err != nil {
		log.Fatal(err)
	}
	defer wm.Close()

	// Instantiate a module named "greet" that imports the "log" function
	// defined in "env".
	greet, err := r.InstantiateModuleFromCodeWithConfig(ctx, greetWasm, wazero.NewModuleConfig().WithName("greet"))
	if err != nil {
		log.Fatal(err)
	}
	defer greet.Close()

	// Get a references to functions we'll use in this example.
	greet := greet.ExportedFunction("greet")
	// These are undocumented, but exported. See tinygo-org/tinygo#2788
	malloc := greet.ExportedFunction("malloc")
	free := greet.ExportedFunction("free")

	// Let's use the argument to this main function in Wasm.
	name := os.Args[1]
	byteCount := uint64(len(name))

	// Instead of an arbitrary memory offset, use TinyGo's allocator. Notice
	// there is nothing string-specific in this allocation function. The same
	// function could be used to pass binary serialized data to Wasm.
	results, err := malloc.Call(ctx, byteCount)
	if err != nil {
		log.Fatal(err)
	}
	pointer := results[0]
	// This pointer is managed by TinyGo, but TinyGo is unaware of external usage.
	// So, we have to free it when finished
	defer free.Call(ctx, pointer)

	// The pointer is a linear memory offset, which is where we write the name.
	if !greet.Memory().Write(uint32(pointer), []byte(name)) {
		log.Fatalf("Memory.Write(%d, %d) out of range of memory size %d",
			pointer, byteCount, greet.Memory().Size())
	}

	// Now, we can call "greet", which reads the string we wrote to memory!
	_, err = greet.Call(ctx, pointer, byteCount)
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
