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

// ctx is a default context used to avoid lint warnings even though these tests don't use any context data.
var ctx = context.Background()

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
	//go:embed testdata/fib.wasm
	fibWasm []byte
	//go:embed testdata/fac.wasm
	facWasm []byte
	//go:embed testdata/unreachable.wasm
	unreachableWasm []byte
	//go:embed testdata/memory.wasm
	memoryWasm []byte
	//go:embed testdata/recursive.wasm
	recursiveWasm []byte
	//go:embed testdata/hugestack.wasm
	hugestackWasm []byte
)

func runAdhocTests(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	t.Run("huge stack", func(t *testing.T) {
		testHugeStack(t, newRuntimeConfig)
	})
	t.Run("fibonacci", func(t *testing.T) {
		testFibonacci(t, newRuntimeConfig)
	})
	t.Run("fac", func(t *testing.T) {
		testFac(t, newRuntimeConfig)
	})
	t.Run("unreachable", func(t *testing.T) {
		testUnreachable(t, newRuntimeConfig)
	})
	t.Run("memory", func(t *testing.T) {
		testMemory(t, newRuntimeConfig)
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
	module, err := r.NewModuleFromSource(hugestackWasm)
	require.NoError(t, err)

	fn := module.Function("main")
	require.NotNil(t, fn)

	_, err = fn.Call(ctx)
	require.NoError(t, err)
}

func testFibonacci(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	r := wazero.NewRuntimeWithConfig(newRuntimeConfig())
	module, err := r.NewModuleFromSource(fibWasm)
	require.NoError(t, err)

	fib := module.Function("fib")
	require.NotNil(t, fib)

	results, err := fib.Call(ctx, 20)
	require.NoError(t, err)
	require.Equal(t, uint64(10946), results[0])
}

func testFac(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	r := wazero.NewRuntimeWithConfig(newRuntimeConfig())
	module, err := r.NewModuleFromSource(facWasm)
	require.NoError(t, err)
	for _, name := range []string{
		"fac-rec",
		"fac-iter",
		"fac-rec-named",
		"fac-iter-named",
		"fac-opt",
	} {
		name := name

		fac := module.Function("fac")

		t.Run(name, func(t *testing.T) {
			results, err := fac.Call(ctx, 25)
			require.NoError(t, err)
			require.Equal(t, uint64(7034535277573963776), results[0])
		})
	}

	t.Run("fac-rec - stack overflow", func(t *testing.T) {
		_, err := module.Function("fac-rec").Call(ctx, 1073741824)
		require.ErrorIs(t, err, wasm.ErrRuntimeCallStackOverflow)
	})
}

func testUnreachable(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	callUnreachable := func(ctx publicwasm.ModuleContext) {
		panic("panic in host function")
	}

	r := wazero.NewRuntimeWithConfig(newRuntimeConfig())

	hostModule := &wazero.HostModuleConfig{Name: "host", Functions: map[string]interface{}{"cause_unreachable": callUnreachable}}
	_, err := r.NewHostModule(hostModule)
	require.NoError(t, err)

	module, err := r.NewModuleFromSource(unreachableWasm)
	require.NoError(t, err)

	_, err = module.Function("main").Call(ctx)
	exp := `wasm runtime error: panic in host function
wasm backtrace:
	0: host.cause_unreachable
	1: two
	2: one
	3: main`
	require.Equal(t, exp, err.Error())
}

func testMemory(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	r := wazero.NewRuntimeWithConfig(newRuntimeConfig())
	module, err := r.NewModuleFromSource(memoryWasm)
	require.NoError(t, err)

	size := module.Function("size")

	// First, we have zero-length memory instance.
	results, err := size.Call(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(0), results[0])

	grow := module.Function("grow")

	// Then grow the memory.
	newPages := uint64(10)
	results, err = grow.Call(ctx, newPages)
	require.NoError(t, err)

	// Grow returns the previous number of memory pages, namely zero.
	require.Equal(t, uint64(0), results[0])

	// Now size should return the new pages -- 10.
	results, err = size.Call(ctx)
	require.NoError(t, err)
	require.Equal(t, newPages, results[0])

	// Growing memory with zero pages is valid but should be noop.
	results, err = grow.Call(ctx, 0)
	require.NoError(t, err)
	require.Equal(t, newPages, results[0])
}

func testRecursiveEntry(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	hostfunc := func(mod publicwasm.ModuleContext) {
		_, err := mod.Function("called_by_host_func").Call(ctx)
		require.NoError(t, err)
	}

	r := wazero.NewRuntimeWithConfig(newRuntimeConfig())

	hostModule := &wazero.HostModuleConfig{Name: "env", Functions: map[string]interface{}{"host_func": hostfunc}}
	_, err := r.NewHostModule(hostModule)
	require.NoError(t, err)

	module, err := r.NewModuleFromSource(recursiveWasm)
	require.NoError(t, err)

	_, err = module.Function("main").Call(ctx, 1)
	require.NoError(t, err)
}

// testImportedAndExportedFunc fails if the engine cannot call an "imported-and-then-exported-back" function
// Notably, this uses memory, which ensures wasm.ModuleContext is valid in both interpreter and JIT engines.
func testImportedAndExportedFunc(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	var memory *wasm.MemoryInstance
	storeInt := func(ctx publicwasm.ModuleContext, offset uint32, val uint64) uint32 {
		if !ctx.Memory().WriteUint64Le(offset, val) {
			return 1
		}
		// sneak a reference to the memory, so we can check it later
		memory = ctx.Memory().(*wasm.MemoryInstance)
		return 0
	}

	r := wazero.NewRuntimeWithConfig(newRuntimeConfig())

	hostModule := &wazero.HostModuleConfig{Name: "", Functions: map[string]interface{}{"store_int": storeInt}}
	_, err := r.NewHostModule(hostModule)
	require.NoError(t, err)

	module, err := r.NewModuleFromSource([]byte(`(module $test
		(import "" "store_int"
			(func $store_int (param $offset i32) (param $val i64) (result (;errno;) i32)))
		(memory $memory 1 1)
		(export "memory" (memory $memory))
		;; store_int is imported from the environment, but it's also exported back to the environment
		(export "store_int" (func $store_int))
		)`))
	require.NoError(t, err)

	// Call store_int and ensure it didn't return an error code.
	results, err := module.Function("store_int").Call(ctx, 1, math.MaxUint64)
	require.NoError(t, err)
	require.Equal(t, uint64(0), results[0])

	// Since offset=1 and val=math.MaxUint64, we expect to have written exactly 8 bytes, with all bits set, at index 1.
	require.Equal(t, []byte{0x0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x0}, memory.Buffer[0:10])
}

//  testHostFunctions ensures arg0 is optionally a context, and fails if a float parameter corrupts a host function value
func testHostFunctions(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	ctx := context.Background()

	floatFuncs := map[string]interface{}{
		"identity_f32": func(value float32) float32 {
			return value
		},
		"identity_f64": func(value float64) float64 {
			return value
		}}

	floatFuncsGoContext := map[string]interface{}{
		"identity_f32": func(funcCtx context.Context, value float32) float32 {
			require.Equal(t, ctx, funcCtx)
			return value
		},
		"identity_f64": func(funcCtx context.Context, value float64) float64 {
			require.Equal(t, ctx, funcCtx)
			return value
		}}

	floatFuncsModuleContext := map[string]interface{}{
		"identity_f32": func(funcCtx publicwasm.ModuleContext, value float32) float32 {
			require.Equal(t, ctx, funcCtx.Context())
			return value
		},
		"identity_f64": func(funcCtx publicwasm.ModuleContext, value float64) float64 {
			require.Equal(t, ctx, funcCtx.Context())
			return value
		}}

	for k, v := range map[string]map[string]interface{}{
		"":                      floatFuncs,
		" - context.Context":    floatFuncsGoContext,
		" - wasm.ModuleContext": floatFuncsModuleContext,
	} {
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig())

		hostModule := &wazero.HostModuleConfig{Name: "host", Functions: v}
		_, err := r.NewHostModule(hostModule)
		require.NoError(t, err)

		m, err := r.NewModuleFromSource([]byte(`(module $test
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

			results, err := m.Function(name).Call(ctx, publicwasm.EncodeF32(input)) // float bits are a uint32 value, call requires uint64
			require.NoError(t, err)
			require.Equal(t, input, publicwasm.DecodeF32(results[0]))
		})

		t.Run(fmt.Sprintf("host function with f64 param%s", k), func(t *testing.T) {
			name := "call->test.identity_f64"
			input := math.MaxFloat64

			results, err := m.Function(name).Call(ctx, publicwasm.EncodeF64(input))
			require.NoError(t, err)
			require.Equal(t, input, publicwasm.DecodeF64(results[0]))
		})
	}
}
