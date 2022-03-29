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
	publicwasm "github.com/tetratelabs/wazero/wasm"
)

var tests = map[string]func(t *testing.T, r wazero.Runtime){
	"huge stack":                                 testHugeStack,
	"unreachable":                                testUnreachable,
	"recursive entry":                            testRecursiveEntry,
	"imported-and-exported func":                 testImportedAndExportedFunc,
	"host function with float type":              testHostFunctions,
	"close module with in-flight calls":          testCloseInFlight,
	"close imported module with in-flight calls": testCloseImportedInFlight,
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

func runAllTests(t *testing.T, tests map[string]func(t *testing.T, r wazero.Runtime), config *wazero.RuntimeConfig) {
	for name, testf := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			testf(t, wazero.NewRuntimeWithConfig(config))
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

//  testHostFunctions ensures arg0 is optionally a context, and fails if a float parameter corrupts a host function value
func testHostFunctions(t *testing.T, r wazero.Runtime) {
	floatFuncs := func(publicwasm.Module) map[string]interface{} {
		return map[string]interface{}{
			"identity_f32": func(value float32) float32 {
				return value
			},
			"identity_f64": func(value float64) float64 {
				return value
			},
		}
	}

	floatFuncsGoContext := func(publicwasm.Module) map[string]interface{} {
		return map[string]interface{}{
			"identity_f32": func(ctx context.Context, value float32) float32 {
				require.Equal(t, context.Background(), ctx)
				return value
			},
			"identity_f64": func(ctx context.Context, value float64) float64 {
				require.Equal(t, context.Background(), ctx)
				return value
			},
		}
	}

	floatFuncsModule := func(m publicwasm.Module) map[string]interface{} {
		return map[string]interface{}{
			"identity_f32": func(ctx publicwasm.Module, value float32) float32 {
				require.Equal(t, m, ctx)
				return value
			},
			"identity_f64": func(ctx publicwasm.Module, value float64) float64 {
				require.Equal(t, m, ctx)
				return value
			},
		}
	}

	setup := func(suffix string, fns func(publicwasm.Module) map[string]interface{}) (publicwasm.Module, publicwasm.Module) {
		var importing publicwasm.Module
		importedName := "imported" + suffix
		importingName := "importing" + suffix

		imported, err := r.NewModuleBuilder(importedName).ExportFunctions(fns(importing)).Instantiate()
		require.NoError(t, err)

		m, err := r.CompileModule([]byte(fmt.Sprintf(`(module
	;; these imports return the input param
	(import "%[1]s" "identity_f32" (func $test.identity_f32 (param f32) (result f32)))
	(import "%[1]s" "identity_f64" (func $test.identity_f64 (param f64) (result f64)))

	;; 'call->test.identity_fXX' proxies 'test.identity_fXX' to test floats aren't corrupted through OpCodeCall
	(func $call->test.identity_f32 (param f32) (result f32)
		local.get 0
		call $test.identity_f32
	)
	(export "call->test.identity_f32" (func $call->test.identity_f32))
	(func $call->test.identity_f64 (param f64) (result f64)
		local.get 0
		call $test.identity_f64
	)
	(export "call->test.identity_f64" (func $call->test.identity_f64))
)`, importedName)))
		require.NoError(t, err)

		importing, err = r.InstantiateModule(m.WithName(importingName))
		require.NoError(t, err)

		return imported, importing
	}

	for k, v := range map[string]func(publicwasm.Module) map[string]interface{}{
		"":                   floatFuncs,
		" - context.Context": floatFuncsGoContext,
		" - wasm.Module":     floatFuncsModule,
	} {
		k := k
		t.Run(fmt.Sprintf("host function with f32 param%s", k), func(t *testing.T) {
			h, m := setup(k, v)
			defer h.Close()
			defer m.Close()

			name := "call->test.identity_f32"
			input := float32(math.MaxFloat32)

			results, err := m.ExportedFunction(name).Call(nil, publicwasm.EncodeF32(input)) // float bits are a uint32 value, call requires uint64
			require.NoError(t, err)
			require.Equal(t, input, publicwasm.DecodeF32(results[0]))
		})

		t.Run(fmt.Sprintf("host function with f64 param%s", k), func(t *testing.T) {
			h, m := setup(k, v)
			defer h.Close()
			defer m.Close()

			name := "call->test.identity_f64"
			input := math.MaxFloat64

			results, err := m.ExportedFunction(name).Call(nil, publicwasm.EncodeF64(input))
			require.NoError(t, err)
			require.Equal(t, input, publicwasm.DecodeF64(results[0]))
		})
	}
}

func testCloseInFlight(t *testing.T, r wazero.Runtime) {
	var moduleCloser func() error
	_, err := r.NewModuleBuilder("host").ExportFunctions(map[string]interface{}{
		"close_module": func() { _ = moduleCloser() }, // Closing while executing itself.
	}).Instantiate()
	require.NoError(t, err)

	m, err := r.InstantiateModuleFromSource([]byte(`(module $test
	(import "host" "close_module" (func $close_module ))

	(func $close_while_execution
		call $close_module
	)
	(export "close_while_execution" (func $close_while_execution))
)`))
	require.NoError(t, err)

	moduleCloser = m.Close

	_, err = m.ExportedFunction("close_while_execution").Call(nil)
	require.NoError(t, err)

}

func testCloseImportedInFlight(t *testing.T, r wazero.Runtime) {
	importedModule, err := r.NewModuleBuilder("host").ExportFunctions(map[string]interface{}{
		"already_closed": func() {},
	}).Instantiate()
	require.NoError(t, err)

	m, err := r.InstantiateModuleFromSource([]byte(`(module $test
		(import "host" "already_closed" (func $already_closed ))

		(func $close_parent_before_execution
			call $already_closed
		)
		(export "close_parent_before_execution" (func $close_parent_before_execution))
	)`))
	require.NoError(t, err)

	// Closing the imported module before making call should also safe.
	require.NoError(t, importedModule.Close())

	// Even we can re-enstantiate the module for the same name.
	importedModuleNew, err := r.NewModuleBuilder("host").ExportFunctions(map[string]interface{}{
		"already_closed": func() {
			panic("unreachable") // The new module's function must not be called.
		},
	}).Instantiate()
	require.NoError(t, err)
	defer importedModuleNew.Close() // nolint

	_, err = m.ExportedFunction("close_parent_before_execution").Call(nil)
	require.NoError(t, err)
}
