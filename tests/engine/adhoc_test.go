package adhoc

import (
	"context"
	_ "embed"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/sys"
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
		name := name   // pin
		testf := testf // pin
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
	module, err := r.InstantiateModuleFromCode(hugestackWasm)
	require.NoError(t, err)
	defer module.Close()

	fn := module.ExportedFunction("main")
	require.NotNil(t, fn)

	_, err = fn.Call(nil)
	require.NoError(t, err)
}

func testUnreachable(t *testing.T, r wazero.Runtime) {
	callUnreachable := func(nil api.Module) {
		panic("panic in host function")
	}

	_, err := r.NewModuleBuilder("host").ExportFunction("cause_unreachable", callUnreachable).Instantiate()
	require.NoError(t, err)

	module, err := r.InstantiateModuleFromCode(unreachableWasm)
	require.NoError(t, err)
	defer module.Close()

	_, err = module.ExportedFunction("main").Call(nil)
	exp := `panic in host function (recovered by wazero)
wasm stack trace:
	host.cause_unreachable()
	.two()
	.one()
	.main()`
	require.Equal(t, exp, err.Error())
}

func testRecursiveEntry(t *testing.T, r wazero.Runtime) {
	hostfunc := func(mod api.Module) {
		_, err := mod.ExportedFunction("called_by_host_func").Call(nil)
		require.NoError(t, err)
	}

	_, err := r.NewModuleBuilder("env").ExportFunction("host_func", hostfunc).Instantiate()
	require.NoError(t, err)

	module, err := r.InstantiateModuleFromCode(recursiveWasm)
	require.NoError(t, err)
	defer module.Close()

	_, err = module.ExportedFunction("main").Call(nil, 1)
	require.NoError(t, err)
}

func TestImportedAndExportedFunc(t *testing.T) {
	r := wazero.NewRuntime()
	testImportedAndExportedFunc(t, r)
}

// testImportedAndExportedFunc fails if the engine cannot call an "imported-and-then-exported-back" function
// Notably, this uses memory, which ensures api.Module is valid in both interpreter and JIT engines.
func testImportedAndExportedFunc(t *testing.T, r wazero.Runtime) {
	var memory *wasm.MemoryInstance
	storeInt := func(m api.Module, offset uint32, val uint64) uint32 {
		if !m.Memory().WriteUint64Le(offset, val) {
			return 1
		}
		// sneak a reference to the memory, so we can check it later
		memory = m.Memory().(*wasm.MemoryInstance)
		return 0
	}

	host, err := r.NewModuleBuilder("").ExportFunction("store_int", storeInt).Instantiate()
	require.NoError(t, err)
	defer host.Close()

	module, err := r.InstantiateModuleFromCode([]byte(`(module $test
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
	fn := module.ExportedFunction("store_int")
	results, err := fn.Call(nil, 1, math.MaxUint64)
	require.NoError(t, err)
	require.Equal(t, uint64(0), results[0])

	// Since offset=1 and val=math.MaxUint64, we expect to have written exactly 8 bytes, with all bits set, at index 1.
	require.Equal(t, []byte{0x0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x0}, memory.Buffer[0:10])
}

// testHostFunctionContextParameter ensures arg0 is optionally a context.
func testHostFunctionContextParameter(t *testing.T, r wazero.Runtime) {
	importedName := t.Name() + "-imported"
	importingName := t.Name() + "-importing"

	var importing api.Module
	fns := map[string]interface{}{
		"no_context": func(p uint32) uint32 {
			return p + 1
		},
		"go_context": func(ctx context.Context, p uint32) uint32 {
			require.Equal(t, configContext, ctx)
			return p + 1
		},
		"module_context": func(module api.Module, p uint32) uint32 {
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
			importing, err = r.InstantiateModuleFromCode([]byte(fmt.Sprintf(`(module $%[1]s
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
			input:    api.EncodeF32(math.MaxFloat32 - 1),
			expected: api.EncodeF32(math.MaxFloat32),
		},
		{
			name:     "f64",
			input:    api.EncodeF64(math.MaxFloat64 - 1),
			expected: api.EncodeF64(math.MaxFloat64),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Instantiate a module that uses Wasm code to call the host function.
			importing, err := r.InstantiateModuleFromCode([]byte(fmt.Sprintf(`(module $%[1]s
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

func callReturnImportSource(importedModule, importingModule string) []byte {
	return []byte(fmt.Sprintf(`(module $%[1]s
	;; test an imported function by re-exporting it
	(import "%[2]s" "return_input" (func $return_input (param i32) (result i32)))
	(export "return_input" (func $return_input))

	;; test wasm, by calling an imported function
	(func $call_return_import (param i32) (result i32) local.get 0 call $return_input)
	(export "call_return_import" (func $call_return_import))
)`, importingModule, importedModule))
}

func testCloseInFlight(t *testing.T, r wazero.Runtime) {
	tests := []struct {
		name, function                string
		closeImporting, closeImported uint32
	}{
		{ // Ex. WASI proc_exit or AssemblyScript abort handler.
			name:           "importing",
			function:       "call_return_import",
			closeImporting: 1,
		},
		// TODO: A module that re-exports a function (ex "return_input") can call it after it is closed!
		{ // Ex. A function that stops the runtime.
			name:           "both",
			function:       "call_return_import",
			closeImporting: 1,
			closeImported:  2,
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			var imported, importing api.Module
			var err error
			closeAndReturn := func(x uint32) uint32 {
				if tc.closeImporting != 0 {
					require.NoError(t, importing.CloseWithExitCode(tc.closeImporting))
				}
				if tc.closeImported != 0 {
					require.NoError(t, imported.CloseWithExitCode(tc.closeImported))
				}
				return x
			}

			// Create the host module, which exports the function that closes the importing module.
			imported, err = r.NewModuleBuilder(t.Name()+"-imported").
				ExportFunction("return_input", closeAndReturn).Instantiate()
			require.NoError(t, err)
			defer imported.Close()

			// Import that module.
			source := callReturnImportSource(imported.Name(), t.Name()+"-importing")
			importing, err = r.InstantiateModuleFromCode(source)
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
			_, err = importing.ExportedFunction(tc.function).Call(nil, 5)
			require.Equal(t, expectedErr, err)
		})
	}
}
