package experimental_test

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/wasi_snapshot_preview1"
)

// This is a basic example of overriding the file system via WithFS. The main goal is to show how it is configured.
func Example_withFS() {
	ctx := context.Background()

	r := wazero.NewRuntime()
	defer r.Close(ctx) // This closes everything this Runtime created.

	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		log.Panicln(err)
	}

	// Instantiate a module that only re-exports a WASI function that uses the filesystem.
	mod, err := r.InstantiateModuleFromCode(ctx, []byte(`(module
  (import "wasi_snapshot_preview1" "fd_prestat_dir_name"
    (func $wasi.fd_prestat_dir_name (param $fd i32) (param $path i32) (param $path_len i32) (result (;errno;) i32)))

  (memory 1 1) ;; memory is required for WASI

  (export "fd_prestat_dir_name" (func 0))
)`))
	if err != nil {
		log.Panicln(err)
	}

	// Setup the filesystem overlay, noting that it can fail if the directory is invalid and must be closed.
	ctx, closer, err := experimental.WithFS(ctx, os.DirFS("."))
	if err != nil {
		log.Panicln(err)
	}
	defer closer.Close(ctx)

	fdPrestatDirName := mod.ExportedFunction("fd_prestat_dir_name")
	fd := 3         // after stderr
	pathLen := 1    // length we expect the path to be.
	pathOffset := 0 // where to write pathLen bytes.

	// By default, there are no pre-opened directories. If the configuration was wrong, this call would fail.
	results, err := fdPrestatDirName.Call(ctx, uint64(fd), uint64(pathOffset), uint64(pathLen))
	if err != nil {
		log.Panicln(err)
	}
	if results[0] != 0 {
		log.Panicf("received errno %d\n", results[0])
	}

	// Try to read the path!
	if path, ok := mod.Memory().Read(ctx, uint32(pathOffset), uint32(pathLen)); !ok {
		log.Panicf("Memory.Read(%d,%d) out of range of memory size %d", pathOffset, pathLen, mod.Memory().Size(ctx))
	} else {
		fmt.Println(string(path))
	}

	// Output:
	// /
}
