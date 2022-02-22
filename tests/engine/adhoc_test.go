package adhoc

import (
	"context"
	_ "embed"
	"fmt"
	"math"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero"
	wasm "github.com/tetratelabs/wazero/internal/wasm"
	publicwasm "github.com/tetratelabs/wazero/wasm"
)

func TestJIT(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip()
	}
	runTests(t, wazero.NewEngineJIT)
}

func TestInterpreter(t *testing.T) {
	runTests(t, wazero.NewEngineInterpreter)
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
)

func runTests(t *testing.T, newEngine func() *wazero.Engine) {
	t.Run("fibonacci", func(t *testing.T) {
		testFibonacci(t, newEngine)
	})
	t.Run("fac", func(t *testing.T) {
		testFac(t, newEngine)
	})
	t.Run("unreachable", func(t *testing.T) {
		testUnreachable(t, newEngine)
	})
	t.Run("memory", func(t *testing.T) {
		testMemory(t, newEngine)
	})
	t.Run("recursive entry", func(t *testing.T) {
		testRecursiveEntry(t, newEngine)
	})
	t.Run("imported-and-exported func", func(t *testing.T) {
		testImportedAndExportedFunc(t, newEngine)
	})
	t.Run("host function with float type", func(t *testing.T) {
		testHostFunctions(t, newEngine)
	})
}

func testFibonacci(t *testing.T, newEngine func() *wazero.Engine) {
	mod, err := wazero.DecodeModuleBinary(fibWasm)
	require.NoError(t, err)

	// We execute 1000 times in order to ensure the JIT engine is stable under high concurrency
	// and we have no conflict with Go's runtime.
	const goroutines = 1000
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			store := wazero.NewStoreWithConfig(&wazero.StoreConfig{Engine: newEngine()})
			exports, err := wazero.InstantiateModule(store, mod)
			require.NoError(t, err)

			results, err := exports.Function("fib")(nil, 20)
			require.NoError(t, err)

			require.Equal(t, uint64(10946), results[0])
		}()
	}
	wg.Wait()
}

func testFac(t *testing.T, newEngine func() *wazero.Engine) {
	mod, err := wazero.DecodeModuleBinary(facWasm)
	require.NoError(t, err)
	store := wazero.NewStoreWithConfig(&wazero.StoreConfig{Engine: newEngine()})
	exports, err := wazero.InstantiateModule(store, mod)
	require.NoError(t, err)
	for _, name := range []string{
		"fac-rec",
		"fac-iter",
		"fac-rec-named",
		"fac-iter-named",
		"fac-opt",
	} {
		name := name

		fac := exports.Function("fac")

		t.Run(name, func(t *testing.T) {
			results, err := fac(nil, 25)
			require.NoError(t, err)
			require.Equal(t, uint64(7034535277573963776), results[0])
		})
	}

	t.Run("fac-rec - stack overflow", func(t *testing.T) {
		_, err := exports.Function("fac-rec")(nil, 1073741824)
		require.ErrorIs(t, err, wasm.ErrRuntimeCallStackOverflow)
	})
}

func testUnreachable(t *testing.T, newEngine func() *wazero.Engine) {
	mod, err := wazero.DecodeModuleBinary(unreachableWasm)
	require.NoError(t, err)

	callUnreachable := func(ctx publicwasm.ModuleContext) {
		_, err := ctx.Function("unreachable_func")(ctx.Context())
		require.NoError(t, err)
	}

	store := wazero.NewStoreWithConfig(&wazero.StoreConfig{Engine: newEngine()})

	_, err = wazero.ExportHostFunctions(store, "host", map[string]interface{}{"cause_unreachable": callUnreachable})
	require.NoError(t, err)

	exports, err := wazero.InstantiateModule(store, mod)
	require.NoError(t, err)

	_, err = exports.Function("main")(nil)
	exp := `wasm runtime error: unreachable
wasm backtrace:
	0: unreachable_func
	1: host.cause_unreachable
	2: two
	3: one
	4: main`
	require.ErrorIs(t, err, wasm.ErrRuntimeUnreachable)
	require.Equal(t, exp, err.Error())
}

func testMemory(t *testing.T, newEngine func() *wazero.Engine) {
	mod, err := wazero.DecodeModuleBinary(memoryWasm)
	require.NoError(t, err)
	store := wazero.NewStoreWithConfig(&wazero.StoreConfig{Engine: newEngine()})
	exports, err := wazero.InstantiateModule(store, mod)
	require.NoError(t, err)

	size := exports.Function("size")

	// First, we have zero-length memory instance.
	results, err := size(nil)
	require.NoError(t, err)
	require.Equal(t, uint64(0), results[0])

	grow := exports.Function("grow")

	// Then grow the memory.
	newPages := uint64(10)
	results, err = grow(nil, newPages)
	require.NoError(t, err)

	// Grow returns the previous number of memory pages, namely zero.
	require.Equal(t, uint64(0), results[0])

	// Now size should return the new pages -- 10.
	results, err = size(nil)
	require.NoError(t, err)
	require.Equal(t, newPages, results[0])

	// Growing memory with zero pages is valid but should be noop.
	results, err = grow(nil, 0)
	require.NoError(t, err)
	require.Equal(t, newPages, results[0])
}

func testRecursiveEntry(t *testing.T, newEngine func() *wazero.Engine) {
	mod, err := wazero.DecodeModuleBinary(recursiveWasm)
	require.NoError(t, err)

	hostfunc := func(ctx publicwasm.ModuleContext) {
		_, err = ctx.Function("called_by_host_func")(nil)
		require.NoError(t, err)
	}

	store := wazero.NewStoreWithConfig(&wazero.StoreConfig{Engine: newEngine()})

	_, err = wazero.ExportHostFunctions(store, "env", map[string]interface{}{"host_func": hostfunc})
	require.NoError(t, err)

	exports, err := wazero.InstantiateModule(store, mod)
	require.NoError(t, err)

	_, err = exports.Function("main")(nil, 1)
	require.NoError(t, err)
}

// testImportedAndExportedFunc fails if the engine cannot call an "imported-and-then-exported-back" function
// Notably, this uses memory, which ensures wasm.ModuleContext is valid in both interpreter and JIT engines.
func testImportedAndExportedFunc(t *testing.T, newEngine func() *wazero.Engine) {
	mod, err := wazero.DecodeModuleText([]byte(`(module $test
		(import "" "store_int"
			(func $store_int (param $offset i32) (param $val i64) (result (;errno;) i32)))
		(memory $memory 1 1)
		(export "memory" (memory $memory))
		;; store_int is imported from the environment, but it's also exported back to the environment
		(export "store_int" (func $store_int))
		)`))
	require.NoError(t, err)

	var memory *wasm.MemoryInstance
	storeInt := func(ctx publicwasm.ModuleContext, offset uint32, val uint64) uint32 {
		if !ctx.Memory().WriteUint64Le(offset, val) {
			return 1
		}
		// sneak a reference to the memory, so we can check it later
		memory = ctx.Memory().(*wasm.MemoryInstance)
		return 0
	}

	store := wazero.NewStoreWithConfig(&wazero.StoreConfig{Engine: newEngine()})

	_, err = wazero.ExportHostFunctions(store, "", map[string]interface{}{"store_int": storeInt})
	require.NoError(t, err)

	exports, err := wazero.InstantiateModule(store, mod)
	require.NoError(t, err)

	// Call store_int and ensure it didn't return an error code.
	results, err := exports.Function("store_int")(nil, 1, math.MaxUint64)
	require.NoError(t, err)
	require.Equal(t, uint64(0), results[0])

	// Since offset=1 and val=math.MaxUint64, we expect to have written exactly 8 bytes, with all bits set, at index 1.
	require.Equal(t, []byte{0x0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x0}, memory.Buffer[0:10])
}

//  testHostFunctions ensures arg0 is optionally a context, and fails if a float parameter corrupts a host function value
func testHostFunctions(t *testing.T, newEngine func() *wazero.Engine) {
	ctx := context.Background()
	mod, err := wazero.DecodeModuleText([]byte(`(module $test
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
		store := wazero.NewStoreWithConfig(&wazero.StoreConfig{Engine: newEngine()})

		_, err = wazero.ExportHostFunctions(store, "host", v)
		require.NoError(t, err)

		m, err := wazero.InstantiateModule(store, mod)
		require.NoError(t, err)

		t.Run(fmt.Sprintf("host function with f32 param%s", k), func(t *testing.T) {
			name := "call->test.identity_f32"
			input := float32(math.MaxFloat32)

			results, err := m.Function(name)(ctx, publicwasm.EncodeF32(input)) // float bits are a uint32 value, call requires uint64
			require.NoError(t, err)
			require.Equal(t, input, publicwasm.DecodeF32(results[0]))
		})

		t.Run(fmt.Sprintf("host function with f64 param%s", k), func(t *testing.T) {
			name := "call->test.identity_f64"
			input := math.MaxFloat64

			results, err := m.Function(name)(ctx, publicwasm.EncodeF64(input))
			require.NoError(t, err)
			require.Equal(t, input, publicwasm.DecodeF64(results[0]))
		})
	}
}
