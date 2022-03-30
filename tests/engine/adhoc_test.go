package adhoc

import (
	"context"
	_ "embed"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero"
	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/sys"
	publicwasm "github.com/tetratelabs/wazero/wasm"
)

var tests = map[string]func(t *testing.T, r wazero.Runtime){
	"huge stack":                           testHugeStack,
	"unreachable":                          testUnreachable,
	"recursive entry":                      testRecursiveEntry,
	"imported-and-exported func":           testImportedAndExportedFunc,
	"host function with context parameter": testHostFunctionContextParameter,
	"host function with numeric parameter": testHostFunctionNumericParameter,
	"close module with in-flight calls":    testCloseInFlight,
}

func TestEngineJIT(t *testing.T) {
	if !wazero.JITSupported {
		t.Skip()
	}
	runAllTests(t, tests, wazero.NewRuntimeConfigJIT())
}

func TestEngineInterpreter(t *testing.T) {
	runAllTests(t, tests, wazero.NewRuntimeConfigInterpreter())
}

type configContextKey string

var configContext = context.WithValue(context.Background(), configContextKey("wa"), "zero")

func runAllTests(t *testing.T, tests map[string]func(t *testing.T, r wazero.Runtime), config *wazero.RuntimeConfig) {
	for name, testf := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			testf(t, wazero.NewRuntimeWithConfig(config.WithContext(configContext)))
		})
	}
}

var (
	//go:embed testdata/unreachable.wasm
	unreachableWasm []byte
	//go:embed testdata/recursive.wasm
	recursiveWasm []byte
	//go:embed testdata/hugestack.wasm
	hugestackWasm []byte
)

func testHugeStack(t *testing.T, r wazero.Runtime) {
	module, err := r.InstantiateModuleFromSource(hugestackWasm)
	require.NoError(t, err)
	defer module.Close()

	fn := module.ExportedFunction("main")
	require.NotNil(t, fn)

	_, err = fn.Call(nil)
	require.NoError(t, err)
}

func testUnreachable(t *testing.T, r wazero.Runtime) {
	callUnreachable := func(nil publicwasm.Module) {
		panic("panic in host function")
	}

	_, err := r.NewModuleBuilder("host").ExportFunction("cause_unreachable", callUnreachable).Instantiate()
	require.NoError(t, err)

	module, err := r.InstantiateModuleFromSource(unreachableWasm)
	require.NoError(t, err)
	defer module.Close()

	_, err = module.ExportedFunction("main").Call(nil)
	exp := `wasm runtime error: panic in host function
wasm backtrace:
	0: cause_unreachable
	1: two
	2: one
	3: main`
	require.Equal(t, exp, err.Error())
}

func testRecursiveEntry(t *testing.T, r wazero.Runtime) {
	hostfunc := func(mod publicwasm.Module) {
		_, err := mod.ExportedFunction("called_by_host_func").Call(nil)
		require.NoError(t, err)
	}

	_, err := r.NewModuleBuilder("env").ExportFunction("host_func", hostfunc).Instantiate()
	require.NoError(t, err)

	module, err := r.InstantiateModuleFromSource(recursiveWasm)
	require.NoError(t, err)
	defer module.Close()

	_, err = module.ExportedFunction("main").Call(nil, 1)
	require.NoError(t, err)
}

// testImportedAndExportedFunc fails if the engine cannot call an "imported-and-then-exported-back" function
// Notably, this uses memory, which ensures wasm.Module is valid in both interpreter and JIT engines.
func testImportedAndExportedFunc(t *testing.T, r wazero.Runtime) {
	var memory *wasm.MemoryInstance
	storeInt := func(nil publicwasm.Module, offset uint32, val uint64) uint32 {
		if !nil.Memory().WriteUint64Le(offset, val) {
			return 1
		}
		// sneak a reference to the memory, so we can check it later
		memory = nil.Memory().(*wasm.MemoryInstance)
		return 0
	}

	_, err := r.NewModuleBuilder("").ExportFunction("store_int", storeInt).Instantiate()
	require.NoError(t, err)

	module, err := r.InstantiateModuleFromSource([]byte(`(module $test
		(import "" "store_int"
			(func $store_int (param $offset i32) (param $val i64) (result (;errno;) i32)))
		(memory $memory 1 1)
		(export "memory" (memory $memory))
		;; store_int is imported from the environment, but it's also exported back to the environment
		(export "store_int" (func $store_int))
		)`))
	require.NoError(t, err)
	defer module.Close()

	// Call store_int and ensure it didn't return an error code.
	results, err := module.ExportedFunction("store_int").Call(nil, 1, math.MaxUint64)
	require.NoError(t, err)
	require.Equal(t, uint64(0), results[0])

	// Since offset=1 and val=math.MaxUint64, we expect to have written exactly 8 bytes, with all bits set, at index 1.
	require.Equal(t, []byte{0x0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x0}, memory.Buffer[0:10])
}

// testHostFunctionContextParameter ensures arg0 is optionally a context.
func testHostFunctionContextParameter(t *testing.T, r wazero.Runtime) {
	importedName := t.Name() + "-imported"
	importingName := t.Name() + "-importing"

	var importing publicwasm.Module
	fns := map[string]interface{}{
		"no_context": func(p uint32) uint32 {
			return p + 1
		},
		"go_context": func(ctx context.Context, p uint32) uint32 {
			require.Equal(t, configContext, ctx)
			return p + 1
		},
		"module_context": func(module publicwasm.Module, p uint32) uint32 {
			require.Equal(t, importing, module)
			return p + 1
		},
	}

	imported, err := r.NewModuleBuilder(importedName).ExportFunctions(fns).Instantiate()
	require.NoError(t, err)
	defer imported.Close()

	for test := range fns {
		t.Run(test, func(t *testing.T) {
			// Instantiate a module that uses Wasm code to call the host function.
			importing, err = r.InstantiateModuleFromSource([]byte(fmt.Sprintf(`(module $%[1]s
	(import "%[2]s" "%[3]s" (func $%[3]s (param i32) (result i32)))
	(func $call_%[3]s (param i32) (result i32) local.get 0 call $%[3]s)
	(export "call->%[3]s" (func $call_%[3]s))
)`, importingName, importedName, test)))
			require.NoError(t, err)
			defer importing.Close()

			results, err := importing.ExportedFunction("call->"+test).Call(nil, math.MaxUint32-1)
			require.NoError(t, err)
			require.Equal(t, uint64(math.MaxUint32), results[0])
		})
	}
}

// testHostFunctionNumericParameter ensures numeric parameters aren't corrupted
func testHostFunctionNumericParameter(t *testing.T, r wazero.Runtime) {
	importedName := t.Name() + "-imported"
	importingName := t.Name() + "-importing"

	fns := map[string]interface{}{
		"i32": func(p uint32) uint32 {
			return p + 1
		},
		"i64": func(p uint64) uint64 {
			return p + 1
		},
		"f32": func(p float32) float32 {
			return p + 1
		},
		"f64": func(p float64) float64 {
			return p + 1
		},
	}

	imported, err := r.NewModuleBuilder(importedName).ExportFunctions(fns).Instantiate()
	require.NoError(t, err)
	defer imported.Close()

	for _, test := range []struct {
		name            string
		input, expected uint64
	}{
		{
			name:     "i32",
			input:    math.MaxUint32 - 1,
			expected: math.MaxUint32,
		},
		{
			name:     "i64",
			input:    math.MaxUint64 - 1,
			expected: math.MaxUint64,
		},
		{
			name:     "f32",
			input:    publicwasm.EncodeF32(math.MaxFloat32 - 1),
			expected: publicwasm.EncodeF32(math.MaxFloat32),
		},
		{
			name:     "f64",
			input:    publicwasm.EncodeF64(math.MaxFloat64 - 1),
			expected: publicwasm.EncodeF64(math.MaxFloat64),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Instantiate a module that uses Wasm code to call the host function.
			importing, err := r.InstantiateModuleFromSource([]byte(fmt.Sprintf(`(module $%[1]s
	(import "%[2]s" "%[3]s" (func $%[3]s (param %[3]s) (result %[3]s)))
	(func $call_%[3]s (param %[3]s) (result %[3]s) local.get 0 call $%[3]s)
	(export "call->%[3]s" (func $call_%[3]s))
)`, importingName, importedName, test.name)))
			require.NoError(t, err)
			defer importing.Close()

			results, err := importing.ExportedFunction("call->"+test.name).Call(nil, test.input)
			require.NoError(t, err)
			require.Equal(t, test.expected, results[0])
		})
	}
}

func callImportAfterAddSource(importedModule, importingModule string) []byte {
	return []byte(fmt.Sprintf(`(module $%[1]s
	(import "%[2]s" "return_input" (func $block (param i32) (result i32)))
	(func $call_import_after_add (param i32) (param i32) (result i32)
		local.get 0
		local.get 1
		i32.add
		call $block
	)
	(export "call_import_after_add" (func $call_import_after_add))
)`, importingModule, importedModule))
}

func testCloseInFlight(t *testing.T, r wazero.Runtime) {
	tests := []struct {
		name                          string
		closeImporting, closeImported uint32
	}{
		{
			name:           "importing", // Ex. WASI proc_exit or AssemblyScript abort handler.
			closeImporting: 1,
		},
		{
			name:          "imported",
			closeImported: 2,
		},
		{
			name:           "both", // Ex. A function that stops the runtime.
			closeImporting: 1,
			closeImported:  2,
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			var imported, importing publicwasm.Module
			var err error
			closeAndReturn := func(x uint32) uint32 {
				if tc.closeImporting != 0 {
					require.NoError(t, importing.CloseWithExitCode(tc.closeImporting))
				}
				if tc.closeImported != 0 {
					require.NoError(t, importing.CloseWithExitCode(tc.closeImported))
				}
				return x
			}

			// Create the host module, which exports the function that closes the importing module.
			imported, err = r.NewModuleBuilder(t.Name()+"-imported").
				ExportFunction("return_input", closeAndReturn).Instantiate()
			require.NoError(t, err)
			defer imported.Close()

			// Import that module.
			source := callImportAfterAddSource(imported.Name(), t.Name()+"-importing")
			importing, err = r.InstantiateModuleFromSource(source)
			require.NoError(t, err)
			defer importing.Close()

			var expectedErr error
			if tc.closeImported != 0 && tc.closeImporting != 0 {
				// When both modules are closed, importing is the better one to choose in the error message.
				expectedErr = sys.NewExitError(importing.Name(), tc.closeImporting)
			} else if tc.closeImported != 0 {
				expectedErr = sys.NewExitError(imported.Name(), tc.closeImported)
			} else if tc.closeImporting != 0 {
				expectedErr = sys.NewExitError(importing.Name(), tc.closeImporting)
			} else {
				t.Fatal("invalid test case")
			}

			// Functions that return after being closed should have an exit error.
			_, err = importing.ExportedFunction("call_import_after_add").Call(nil, 1, 2)
			require.Equal(t, expectedErr, err)

			// New function calls after being closed should have the same exit error.
			_, err = importing.ExportedFunction("call_import_after_add").Call(nil, 1, 2)
			require.Equal(t, expectedErr, err)
		})
	}
}
