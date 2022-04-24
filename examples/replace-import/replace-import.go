package replace_import

import (
	"context"
	_ "embed"
	"fmt"
	"log"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// main shows how to override a module or function name hard-coded in a
// WebAssembly module. This is similar to what some tools call "linking".
func main() {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime()

	// Instantiate a Go-defined module named "assemblyscript" that exports a
	// function to close the module that calls "abort".
	host, err := r.NewModuleBuilder("assemblyscript").
		ExportFunction("abort", func(m api.Module, messageOffset, fileNameOffset, line, col uint32) {
			_ = m.CloseWithExitCode(255)
		}).Instantiate(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer host.Close()

	// Compile WebAssembly code that needs the function "env.abort".
	code, err := r.CompileModule(ctx, []byte(`(module $needs-import
	(import "env" "abort" (func $~lib/builtins/abort (param i32 i32 i32 i32)))

	(export "abort" (func 0)) ;; exports the import for testing
)`))
	if err != nil {
		log.Fatal(err)
	}
	defer code.Close()

	// Instantiate the WebAssembly module, replacing the import "env.abort"
	// with "assemblyscript.abort".
	mod, err := r.InstantiateModuleWithConfig(ctx, code, wazero.NewModuleConfig().
		WithImport("env", "abort", "assemblyscript", "abort"))
	if err != nil {
		log.Fatal(err)
	}
	defer mod.Close()

	// Since the above worked, the exported function closes the module.
	_, err = mod.ExportedFunction("abort").Call(ctx, 0, 0, 0, 0)
	fmt.Println(err)
}
