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
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Instantiate a Go-defined module named "assemblyscript" that exports a
	// function to close the module that calls "abort".
	host, err := r.NewModuleBuilder("assemblyscript").
		ExportFunction("abort", func(ctx context.Context, m api.Module, messageOffset, fileNameOffset, line, col uint32) {
			_ = m.CloseWithExitCode(ctx, 255)
		}).Instantiate(ctx)
	if err != nil {
		log.Panicln(err)
	}
	defer host.Close(ctx)

	// Compile the WebAssembly module, replacing the import "env.abort" with "assemblyscript.abort".
	compileConfig := wazero.NewCompileConfig().
		WithImportRenamer(func(externType api.ExternType, oldModule, oldName string) (newModule, newName string) {
			if oldModule == "env" && oldName == "abort" {
				return "assemblyscript", "abort"
			}
			return oldModule, oldName
		})

	code, err := r.CompileModule(ctx, []byte(`(module $needs-import
	(import "env" "abort" (func $~lib/builtins/abort (param i32 i32 i32 i32)))

	(export "abort" (func 0)) ;; exports the import for testing
)`), compileConfig)
	if err != nil {
		log.Panicln(err)
	}
	defer code.Close(ctx)

	// Instantiate the WebAssembly module.
	mod, err := r.InstantiateModule(ctx, code, wazero.NewModuleConfig())
	if err != nil {
		log.Panicln(err)
	}
	defer mod.Close(ctx)

	// Since the above worked, the exported function closes the module.
	_, err = mod.ExportedFunction("abort").Call(ctx, 0, 0, 0, 0)
	fmt.Println(err)
}
