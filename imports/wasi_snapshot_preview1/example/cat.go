package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// catFS is an embedded filesystem limited to test.txt
//
//go:embed testdata/test.txt
var catFS embed.FS

// catWasmCargoWasi was compiled from testdata/cargo-wasi/cat.rs
//
//go:embed testdata/cargo-wasi/cat.wasm
var catWasmCargoWasi []byte

// catWasmTinyGo was compiled from testdata/tinygo/cat.go
//
//go:embed testdata/tinygo/cat.wasm
var catWasmTinyGo []byte

// catWasmZig was compiled from testdata/zig/cat.zig
//
//go:embed testdata/zig/cat.wasm
var catWasmZig []byte

// catWasmZigCc was compiled from testdata/zig-cc/cat.c
//
//go:embed testdata/zig-cc/cat.wasm
var catWasmZigCc []byte

// main writes an input file to stdout, just like `cat`.
//
// This is a basic introduction to the WebAssembly System Interface (WASI).
// See https://github.com/WebAssembly/WASI
func main() {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Since wazero uses fs.FS, we can use standard libraries to do things like trim the leading path.
	rooted, err := fs.Sub(catFS, "testdata")
	if err != nil {
		log.Panicln(err)
	}

	// Combine the above into our baseline config, overriding defaults.
	config := wazero.NewModuleConfig().
		// By default, I/O streams are discarded and there's no file system.
		WithStdout(os.Stdout).WithStderr(os.Stderr).WithFS(rooted)

	// Instantiate WASI, which implements system I/O such as console output.
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	// Choose the binary we want to test. Most compilers that implement WASI
	// are portable enough to use binaries interchangeably.
	var catWasm []byte
	toolchain := os.Getenv("TOOLCHAIN")
	switch toolchain {
	case "":
		fallthrough // default to TinyGo
	case "cargo-wasi":
		catWasm = catWasmCargoWasi
	case "tinygo":
		catWasm = catWasmTinyGo
	case "zig":
		catWasm = catWasmZig
	case "zig-cc":
		catWasm = catWasmZigCc
	default:
		log.Panicln("unknown toolchain", toolchain)
	}

	// InstantiateModule runs the "_start" function, WASI's "main".
	// * Set the program name (arg[0]) to "wasi"; arg[1] should be "/test.txt".
	if _, err = r.InstantiateWithConfig(ctx, catWasm, config.WithArgs("wasi", os.Args[1])); err != nil {
		// Note: Most compilers do not exit the module after running "_start",
		// unless there was an error. This allows you to call exported functions.
		if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
			fmt.Fprintf(os.Stderr, "exit_code: %d\n", exitErr.ExitCode())
		} else if !ok {
			log.Panicln(err)
		}
	}
}
