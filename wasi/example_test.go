package wasi

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/sys"
)

// This is an example of how to use WebAssembly System Interface (WASI) with its simplest function: "proc_exit".
//
// See https://github.com/tetratelabs/wazero/tree/main/examples/wasi for another example.
func Example() {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime()

	// Instantiate WASI, which implements system I/O such as console output.
	wm, err := InstantiateSnapshotPreview1(ctx, r)
	if err != nil {
		log.Panicln(err)
	}
	defer wm.Close(testCtx)

	// Compile the WebAssembly module using the default configuration.
	code, err := r.CompileModule(ctx, []byte(`
(module
  (import "wasi_snapshot_preview1" "proc_exit" (func $wasi.proc_exit (param $rval i32)))

  (func $main
     i32.const 2           ;; push $rval onto the stack
     call $wasi.proc_exit  ;; return a sys.ExitError to the caller
  )
  (export "_start" (func $main))
)
`), wazero.NewCompileConfig())
	if err != nil {
		log.Panicln(err)
	}
	defer code.Close(ctx)

	// InstantiateModule runs the "_start" function which is like a "main" function.
	// Override default configuration (which discards stdout).
	mod, err := r.InstantiateModule(ctx, code, wazero.NewModuleConfig().WithStdout(os.Stdout).WithName("wasi-demo"))
	if mod != nil {
		defer mod.Close(ctx)
	}
	// Print the exit code
	if exitErr, ok := err.(*sys.ExitError); ok {
		fmt.Printf("exit_code: %d\n", exitErr.ExitCode())
	}

	// Output:
	// exit_code: 2
}
