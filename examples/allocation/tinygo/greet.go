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

// greetWasm was compiled using `tinygo build -o greet.wasm -scheduler=none --no-debug -target=wasi greet.go`
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
	mod, err := r.InstantiateModuleFromCode(ctx, greetWasm)
	if err != nil {
		log.Fatal(err)
	}
	defer mod.Close()

	// Get a references to functions we'll use in this example.
	greet := mod.ExportedFunction("greet")
	greeting := mod.ExportedFunction("greeting")
	// These are undocumented, but exported. See tinygo-org/tinygo#2788
	malloc := mod.ExportedFunction("malloc")
	free := mod.ExportedFunction("free")

	// Let's use the argument to this main function in Wasm.
	name := os.Args[1]
	nameSize := uint64(len(name))

	// Instead of an arbitrary memory offset, use TinyGo's allocator. Notice
	// there is nothing string-specific in this allocation function. The same
	// function could be used to pass binary serialized data to Wasm.
	results, err := malloc.Call(ctx, nameSize)
	if err != nil {
		log.Fatal(err)
	}
	namePtr := results[0]
	// This pointer is managed by TinyGo, but TinyGo is unaware of external usage.
	// So, we have to free it when finished
	defer free.Call(ctx, namePtr)

	// The pointer is a linear memory offset, which is where we write the name.
	if !mod.Memory().Write(uint32(namePtr), []byte(name)) {
		log.Fatalf("Memory.Write(%d, %d) out of range of memory size %d",
			namePtr, nameSize, mod.Memory().Size())
	}

	// Now, we can call "greet", which reads the string we wrote to memory!
	_, err = greet.Call(ctx, namePtr, nameSize)
	if err != nil {
		log.Fatal(err)
	}

	// Finally, we get the greeting message "greet" printed. This shows how to
	// read-back something allocated by TinyGo.
	ptrSize, err := greeting.Call(ctx, namePtr, nameSize)
	if err != nil {
		log.Fatal(err)
	}
	// Note: This pointer is still owned by TinyGo, so don't try to free it!
	greetingPtr := uint32(ptrSize[0] >> 32)
	greetingSize := uint32(ptrSize[0])
	// The pointer is a linear memory offset, which is where we write the name.
	if bytes, ok := mod.Memory().Read(greetingPtr, greetingSize); !ok {
		log.Fatalf("Memory.Read(%d, %d) out of range of memory size %d",
			greetingPtr, greetingSize, mod.Memory().Size())
	} else {
		fmt.Println("go >>", string(bytes))
	}
}

func logString(m api.Module, offset, byteCount uint32) {
	buf, ok := m.Memory().Read(offset, byteCount)
	if !ok {
		log.Fatalf("Memory.Read(%d, %d) out of range", offset, byteCount)
	}
	fmt.Println(string(buf))
}
