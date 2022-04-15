package wasi

import (
	_ "embed"
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
	r := wazero.NewRuntime()

	// Instantiate WASI, which implements system I/O such as console output.
	wm, err := InstantiateSnapshotPreview1(r)
	if err != nil {
		log.Fatal(err)
	}
	defer wm.Close()

	// Override default configuration (which discards stdout).
	config := wazero.NewModuleConfig().WithStdout(os.Stdout)

	// InstantiateModuleFromCodeWithConfig runs the "_start" function which is like a "main" function.
	_, err = r.InstantiateModuleFromCodeWithConfig([]byte(`
(module
  (import "wasi_snapshot_preview1" "proc_exit" (func $wasi.proc_exit (param $rval i32)))

  (func $main
     i32.const 2           ;; push $rval onto the stack
     call $wasi.proc_exit  ;; return a sys.ExitError to the caller
  )
  (export "_start" (func $main))
)
`), config.WithName("wasi-demo"))

	// Print the exit code
	if exitErr, ok := err.(*sys.ExitError); ok {
		fmt.Printf("exit_code: %d\n", exitErr.ExitCode())
	}

	// Output:
	// exit_code: 2
}
