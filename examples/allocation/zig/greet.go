package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// greetWasm was compiled using `zig build`
//
//go:embed testdata/greet.wasm
var greetWasm []byte

// main shows how to interact with a WebAssembly function that was compiled from Zig.
//
// See README.md for a full description.
func main() {
	if err := run(); err != nil {
		log.Panicln(err)
	}
}

func run() error {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Instantiate a Go-defined module named "env" that exports a function to
	// log to the console.
	_, err := r.NewHostModuleBuilder("env").
		NewFunctionBuilder().WithFunc(logString).Export("log").
		Instantiate(ctx)
	if err != nil {
		return err
	}

	// Instantiate a WebAssembly module that imports the "log" function defined
	// in "env" and exports "memory" and functions we'll use in this example.
	mod, err := r.InstantiateWithConfig(ctx, greetWasm,
		wazero.NewModuleConfig().WithStdout(os.Stdout).WithStderr(os.Stderr))
	if err != nil {
		return err
	}

	// Get references to WebAssembly functions we'll use in this example.
	greet := mod.ExportedFunction("greet")
	greeting := mod.ExportedFunction("greeting")

	malloc := mod.ExportedFunction("malloc")
	free := mod.ExportedFunction("free")

	// Let's use the argument to this main function in Wasm.
	name := os.Args[1]
	nameSize := uint64(len(name))

	// Instead of an arbitrary memory offset, use Zig's allocator. Notice
	// there is nothing string-specific in this allocation function. The same
	// function could be used to pass binary serialized data to Wasm.
	results, err := malloc.Call(ctx, nameSize)
	if err != nil {
		return err
	}
	namePtr := results[0]
	if namePtr == 0 {
		return errors.New("malloc failed")
	}
	// We have to free this pointer when finished.
	defer free.Call(ctx, namePtr, nameSize)

	// The pointer is a linear memory offset, which is where we write the name.
	if !mod.Memory().Write(uint32(namePtr), []byte(name)) {
		return fmt.Errorf("Memory.Write(%d, %d) out of range of memory size %d",
			namePtr, nameSize, mod.Memory().Size())
	}

	// Now, we can call "greet", which reads the string we wrote to memory!
	_, err = greet.Call(ctx, namePtr, nameSize)
	if err != nil {
		return err
	}

	// Finally, we get the greeting message "greet" printed. This shows how to
	// read-back something allocated by Zig.
	ptrSize, err := greeting.Call(ctx, namePtr, nameSize)
	if err != nil {
		return err
	}

	greetingPtr := uint32(ptrSize[0] >> 32)
	greetingSize := uint32(ptrSize[0])
	// The pointer is a linear memory offset, which is where we write the name.
	if bytes, ok := mod.Memory().Read(greetingPtr, greetingSize); !ok {
		return fmt.Errorf("Memory.Read(%d, %d) out of range of memory size %d",
			greetingPtr, greetingSize, mod.Memory().Size())
	} else {
		fmt.Println("go >>", string(bytes))
	}

	return nil
}

func logString(_ context.Context, m api.Module, offset, byteCount uint32) {
	buf, ok := m.Memory().Read(offset, byteCount)
	if !ok {
		log.Panicf("Memory.Read(%d, %d) out of range", offset, byteCount)
	}
	fmt.Println(string(buf))
}
