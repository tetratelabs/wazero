package main

import (
	"context"
	"log"
	"os"
	"path"
	"testing/fstest"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental/gojs"
)

// main invokes Wasm compiled via `GOOS=js GOARCH=wasm`, which writes an input
// file to stdout, just like `cat`.
//
// This shows how to integrate a filesystem with wasm using gojs.
func main() {
	// Read the binary compiled with `GOOS=js GOARCH=wasm`.
	bin, err := os.ReadFile(path.Join("cat", "main.wasm"))
	if err != nil {
		log.Panicln(err)
	}

	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Compile the wasm binary to machine code.
	start := time.Now()
	guest, err := r.CompileModule(ctx, bin)
	if err != nil {
		log.Panicln(err)
	}
	compilationTime := time.Since(start).Milliseconds()
	log.Printf("CompileModule took %dms", compilationTime)

	// Instantiate the host functions needed by the guest.
	start = time.Now()
	gojs.MustInstantiate(ctx, r, guest)
	instantiateTime := time.Since(start).Milliseconds()
	log.Printf("gojs.MustInstantiate took %dms", instantiateTime)

	fakeFilesystem := fstest.MapFS{"test.txt": {Data: []byte("greet filesystem\n")}}

	// Create the sandbox configuration used by the guest.
	guestConfig := wazero.NewModuleConfig().
		// By default, I/O streams are discarded and there's no file system.
		WithStdout(os.Stdout).WithStderr(os.Stderr).
		WithFS(fakeFilesystem).
		WithArgs("gojs", os.Args[1]) // only what's in the filesystem will work!

	// Execute the "run" function, which corresponds to "main" in stars/main.go.
	start = time.Now()
	err = gojs.Run(ctx, r, guest, gojs.NewConfig(guestConfig))
	runTime := time.Since(start).Milliseconds()
	log.Printf("gojs.Run took %dms", runTime)
	if err != nil {
		log.Panicln(err)
	}
}
