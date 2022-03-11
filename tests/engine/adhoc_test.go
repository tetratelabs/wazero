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

func TestJITAdhoc(t *testing.T) {
	if !wazero.JITSupported {
		t.Skip()
	}
	runAdhocTests(t, wazero.NewRuntimeConfigJIT)
}

func TestInterpreterAdhoc(t *testing.T) {
	runAdhocTests(t, wazero.NewRuntimeConfigInterpreter)
}

var (
	//go:embed testdata/unreachable.wasm
	unreachableWasm []byte
	//go:embed testdata/recursive.wasm
	recursiveWasm []byte
	//go:embed testdata/hugestack.wasm
	hugestackWasm []byte
)

func runAdhocTests(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	t.Run("huge stack", func(t *testing.T) {
		testHugeStack(t, newRuntimeConfig)
	})
	t.Run("unreachable", func(t *testing.T) {
		testUnreachable(t, newRuntimeConfig)
	})
	t.Run("recursive entry", func(t *testing.T) {
		testRecursiveEntry(t, newRuntimeConfig)
	})
	t.Run("imported-and-exported func", func(t *testing.T) {
		testImportedAndExportedFunc(t, newRuntimeConfig)
	})
	t.Run("host function with float type", func(t *testing.T) {
		testHostFunctions(t, newRuntimeConfig)
	})
}

func testHugeStack(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	r := wazero.NewRuntimeWithConfig(newRuntimeConfig())
	module, err := r.InstantiateModuleFromSource(hugestackWasm)
	require.NoError(t, err)

	fn := module.ExportedFunction("main")
	require.NotNil(t, fn)

	_, err = fn.Call(nil)
	require.NoError(t, err)
}

func testUnreachable(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	callUnreachable := func(nil publicwasm.Module) {
		panic("panic in host function")
	}

	r := wazero.NewRuntimeWithConfig(newRuntimeConfig())

	_, err := r.NewModuleBuilder("host").ExportFunction("cause_unreachable", callUnreachable).Instantiate()
	require.NoError(t, err)

	module, err := r.InstantiateModuleFromSource(unreachableWasm)
	require.NoError(t, err)

	_, err = module.ExportedFunction("main").Call(nil)
	exp := `wasm runtime error: panic in host function
wasm backtrace:
	0: cause_unreachable
	1: two
	2: one
	3: main`
	require.Equal(t, exp, err.Error())
}

func testRecursiveEntry(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	hostfunc := func(mod publicwasm.Module) {
		_, err := mod.ExportedFunction("called_by_host_func").Call(nil)
		require.NoError(t, err)
	}

	r := wazero.NewRuntimeWithConfig(newRuntimeConfig())

	_, err := r.NewModuleBuilder("env").ExportFunction("host_func", hostfunc).Instantiate()
	require.NoError(t, err)

	module, err := r.InstantiateModuleFromSource(recursiveWasm)
	require.NoError(t, err)

	_, err = module.ExportedFunction("main").Call(nil, 1)
	require.NoError(t, err)
}

// testImportedAndExportedFunc fails if the engine cannot call an "imported-and-then-exported-back" function
// Notably, this uses memory, which ensures wasm.Module is valid in both interpreter and JIT engines.
func testImportedAndExportedFunc(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	var memory *wasm.MemoryInstance
	storeInt := func(nil publicwasm.Module, offset uint32, val uint64) uint32 {
		if !nil.Memory().WriteUint64Le(offset, val) {
			return 1
		}
		// sneak a reference to the memory, so we can check it later
		memory = nil.Memory().(*wasm.MemoryInstance)
		return 0
	}

	r := wazero.NewRuntimeWithConfig(newRuntimeConfig())

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

	// Call store_int and ensure it didn't return an error code.
	results, err := module.ExportedFunction("store_int").Call(nil, 1, math.MaxUint64)
	require.NoError(t, err)
	require.Equal(t, uint64(0), results[0])

	// Since offset=1 and val=math.MaxUint64, we expect to have written exactly 8 bytes, with all bits set, at index 1.
	require.Equal(t, []byte{0x0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x0}, memory.Buffer[0:10])
}

func TestHostFunctions(t *testing.T) {
	testHostFunctions(t, func() *wazero.RuntimeConfig {
		return wazero.NewRuntimeConfig()
	})
}

//  testHostFunctions ensures arg0 is optionally a context, and fails if a float parameter corrupts a host function value
func testHostFunctions(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	var m publicwasm.Module

	floatFuncs := map[string]interface{}{
		"identity_f32": func(value float32) float32 {
			return value
		},
		"identity_f64": func(value float64) float64 {
			return value
		}}

	floatFuncsGoContext := map[string]interface{}{
		"identity_f32": func(ctx context.Context, value float32) float32 {
			require.Equal(t, context.Background(), ctx)
			return value
		},
		"identity_f64": func(ctx context.Context, value float64) float64 {
			require.Equal(t, context.Background(), ctx)
			return value
		}}

	floatFuncsModule := map[string]interface{}{
		"identity_f32": func(ctx publicwasm.Module, value float32) float32 {
			require.Equal(t, m, ctx)
			return value
		},
		"identity_f64": func(ctx publicwasm.Module, value float64) float64 {
			require.Equal(t, m, ctx)
			return value
		}}

	for k, v := range map[string]map[string]interface{}{
		"":                   floatFuncs,
		" - context.Context": floatFuncsGoContext,
		" - wasm.Module":     floatFuncsModule,
	} {
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig())

		_, err := r.NewModuleBuilder("host").ExportFunctions(v).Instantiate()
		require.NoError(t, err)

		m, err = r.InstantiateModuleFromSource([]byte(`(module $test
	;; these imports return the input param
	(import "host" "identity_f32" (func $test.identity_f32 (param f32) (result f32)))
	(import "host" "identity_f64" (func $test.identity_f64 (param f64) (result f64)))

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
)`))
		require.NoError(t, err)

		t.Run(fmt.Sprintf("host function with f32 param%s", k), func(t *testing.T) {
			name := "call->test.identity_f32"
			input := float32(math.MaxFloat32)

			results, err := m.ExportedFunction(name).Call(nil, publicwasm.EncodeF32(input)) // float bits are a uint32 value, call requires uint64
			require.NoError(t, err)
			require.Equal(t, input, publicwasm.DecodeF32(results[0]))
		})

		t.Run(fmt.Sprintf("host function with f64 param%s", k), func(t *testing.T) {
			name := "call->test.identity_f64"
			input := math.MaxFloat64

			results, err := m.ExportedFunction(name).Call(nil, publicwasm.EncodeF64(input))
			require.NoError(t, err)
			require.Equal(t, input, publicwasm.DecodeF64(results[0]))
		})
	}
}
