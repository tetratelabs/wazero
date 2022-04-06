package examples

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// Test_Replace shows how you can replace a module import when it doesn't match instantiated modules.
func Test_Replace(t *testing.T) {
	r := wazero.NewRuntime()

	// Instantiate a function that closes the module under "assemblyscript.abort".
	host, err := r.NewModuleBuilder("assemblyscript").
		ExportFunction("abort", func(m api.Module, messageOffset, fileNameOffset, line, col uint32) {
			_ = m.CloseWithExitCode(255)
		}).Instantiate()
	require.NoError(t, err)
	defer host.Close()

	// Compile code that needs the function "env.abort".
	code, err := r.CompileModule([]byte(`(module
	(import "env" "abort" (func $~lib/builtins/abort (param i32 i32 i32 i32)))

	(export "abort" (func 0)) ;; exports the import for testing
)`))
	require.NoError(t, err)

	// Instantiate the module, replacing the import "env.abort" with "assemblyscript.abort".
	mod, err := r.InstantiateModuleWithConfig(code, wazero.NewModuleConfig().
		WithName(t.Name()).
		WithImport("env", "abort", "assemblyscript", "abort"))
	require.NoError(t, err)
	defer mod.Close()

	// Since the above worked, the exported function closes the module.
	_, err = mod.ExportedFunction("abort").Call(nil, 0, 0, 0, 0)
	require.EqualError(t, err, `module "Test_Replace" closed with exit_code(255)`)
}
