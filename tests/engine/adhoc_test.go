package adhoc

import (
	"context"
	_ "embed"
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
		fibonacci(t, newEngine)
	})
	t.Run("fac", func(t *testing.T) {
		fac(t, newEngine)
	})
	t.Run("unreachable", func(t *testing.T) {
		unreachable(t, newEngine)
	})
	t.Run("memory", func(t *testing.T) {
		memory(t, newEngine)
	})
	t.Run("recursive entry", func(t *testing.T) {
		recursiveEntry(t, newEngine)
	})
	t.Run("imported-and-exported func", func(t *testing.T) {
		importedAndExportedFunc(t, newEngine)
	})
	t.Run("host function with float32 type", func(t *testing.T) {
		hostFuncWithFloatParam(t, newEngine)
	})
}

func fibonacci(t *testing.T, newEngine func() *wazero.Engine) {
	ctx := context.Background()
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
			store, err := wazero.NewStoreWithConfig(&wazero.StoreConfig{Engine: newEngine()})
			require.NoError(t, err)
			m, err := store.Instantiate(mod)
			require.NoError(t, err)
			fib, ok := m.GetFunctionI64Return("fib")
			require.True(t, ok)
			out, err := fib(ctx, 20)
			require.NoError(t, err)
			require.Equal(t, uint64(10946), out)
		}()
	}
	wg.Wait()
}

func fac(t *testing.T, newEngine func() *wazero.Engine) {
	ctx := context.Background()
	mod, err := wazero.DecodeModuleBinary(facWasm)
	require.NoError(t, err)
	store, err := wazero.NewStoreWithConfig(&wazero.StoreConfig{Engine: newEngine()})
	require.NoError(t, err)
	m, err := store.Instantiate(mod)
	require.NoError(t, err)
	for _, name := range []string{
		"fac-rec",
		"fac-iter",
		"fac-rec-named",
		"fac-iter-named",
		"fac-opt",
	} {
		name := name
		fac, ok := m.GetFunctionI64Return(name)
		require.True(t, ok)
		t.Run(name, func(t *testing.T) {
			out, err := fac(ctx, 25)
			require.NoError(t, err)
			require.Equal(t, uint64(7034535277573963776), out)
		})
	}
	fac, ok := m.GetFunctionI64Return("fac-rec")
	require.True(t, ok)
	_, err = fac(ctx, 1073741824)
	require.ErrorIs(t, err, wasm.ErrRuntimeCallStackOverflow)
}

func unreachable(t *testing.T, newEngine func() *wazero.Engine) {
	ctx := context.Background()
	mod, err := wazero.DecodeModuleBinary(unreachableWasm)
	require.NoError(t, err)

	callUnreachable := func(ctx publicwasm.HostFunctionCallContext) {
		fn, ok := ctx.Functions().GetFunctionVoidReturn("unreachable_func")
		require.True(t, ok)
		require.NoError(t, fn(ctx.Context()))
	}
	hostFuncs, err := wazero.NewHostFunctions(map[string]interface{}{"cause_unreachable": callUnreachable})
	require.NoError(t, err)

	store, err := wazero.NewStoreWithConfig(&wazero.StoreConfig{
		Engine:                newEngine(),
		ModuleToHostFunctions: map[string]*wazero.HostFunctions{"host": hostFuncs},
	})
	require.NoError(t, err)

	m, err := store.Instantiate(mod)
	require.NoError(t, err)

	main, ok := m.GetFunctionVoidReturn("main")
	require.True(t, ok)

	err = main(ctx)
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

func memory(t *testing.T, newEngine func() *wazero.Engine) {
	ctx := context.Background()
	mod, err := wazero.DecodeModuleBinary(memoryWasm)
	require.NoError(t, err)
	store, err := wazero.NewStoreWithConfig(&wazero.StoreConfig{Engine: newEngine()})
	require.NoError(t, err)
	m, err := store.Instantiate(mod)
	require.NoError(t, err)

	// First, we have zero-length memory instance.
	size, ok := m.GetFunctionI32Return("size")
	require.True(t, ok)
	out, err := size(ctx)
	require.NoError(t, err)
	require.Equal(t, uint32(0), out)

	// Then grow the memory.
	grow, ok := m.GetFunctionI32Return("grow")
	require.True(t, ok)
	newPages := uint32(10)

	out, err = grow(ctx, uint64(newPages))
	require.NoError(t, err)
	// Grow returns the previous number of memory pages, namely zero.
	require.Equal(t, uint32(0), out)

	// Now size should return the new pages -- 10.
	out, err = size(ctx)
	require.NoError(t, err)
	require.Equal(t, newPages, out)

	// Growing memory with zero pages is valid but should be noop.
	out, err = grow(ctx, 0)
	require.NoError(t, err)
	require.Equal(t, newPages, out)
}

func recursiveEntry(t *testing.T, newEngine func() *wazero.Engine) {
	ctx := context.Background()
	mod, err := wazero.DecodeModuleBinary(recursiveWasm)
	require.NoError(t, err)

	hostfunc := func(ctx publicwasm.HostFunctionCallContext) {
		fn, ok := ctx.Functions().GetFunctionI32Return("called_by_host_func")
		require.True(t, ok)
		_, err := fn(ctx.Context())
		require.NoError(t, err)
	}

	hostFuncs, err := wazero.NewHostFunctions(map[string]interface{}{"host_func": hostfunc})
	require.NoError(t, err)

	store, err := wazero.NewStoreWithConfig(&wazero.StoreConfig{
		Engine:                newEngine(),
		ModuleToHostFunctions: map[string]*wazero.HostFunctions{"env": hostFuncs},
	})
	require.NoError(t, err)

	m, err := store.Instantiate(mod)
	require.NoError(t, err)

	main, ok := m.GetFunctionVoidReturn("main")
	require.True(t, ok)

	err = main(ctx, 1)
	require.NoError(t, err)
}

// importedAndExportedFunc fails if the engine cannot call an "imported-and-then-exported-back" function
// Notably, this uses memory, which ensures wasm.HostFunctionCallContext is valid in both interpreter and JIT engines.
func importedAndExportedFunc(t *testing.T, newEngine func() *wazero.Engine) {
	ctx := context.Background()
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
	storeInt := func(ctx publicwasm.HostFunctionCallContext, offset uint32, val uint64) uint32 {
		if !ctx.Memory().WriteUint64Le(offset, val) {
			return 1
		}
		// sneak a reference to the memory so we can check it later
		memory = ctx.Memory().(*wasm.MemoryInstance)
		return 0
	}

	hostFuncs, err := wazero.NewHostFunctions(map[string]interface{}{"store_int": storeInt})
	require.NoError(t, err)

	store, err := wazero.NewStoreWithConfig(&wazero.StoreConfig{
		Engine:                newEngine(),
		ModuleToHostFunctions: map[string]*wazero.HostFunctions{"": hostFuncs},
	})
	require.NoError(t, err)

	m, err := store.Instantiate(mod)
	require.NoError(t, err)

	// Call store_int and ensure it didn't return an error code.
	storeIntFn, ok := m.GetFunctionI32Return("store_int")
	require.True(t, ok)

	result, err := storeIntFn(ctx, 1, math.MaxUint64)
	require.NoError(t, err)
	require.Equal(t, uint32(0), result)

	// Since offset=1 and val=math.MaxUint64, we expect to have written exactly 8 bytes, with all bits set, at index 1.
	require.Equal(t, []byte{0x0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x0}, memory.Buffer[0:10])
}

//  hostFuncWithFloatParam fails if a float parameter corrupts a host function value
func hostFuncWithFloatParam(t *testing.T, newEngine func() *wazero.Engine) {
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

	hostFuncs, err := wazero.NewHostFunctions(map[string]interface{}{
		"identity_f32": func(ctx publicwasm.HostFunctionCallContext, value float32) float32 {
			return value
		},
		"identity_f64": func(ctx publicwasm.HostFunctionCallContext, value float64) float64 {
			return value
		}})
	require.NoError(t, err)

	store, err := wazero.NewStoreWithConfig(&wazero.StoreConfig{
		Engine:                newEngine(),
		ModuleToHostFunctions: map[string]*wazero.HostFunctions{"host": hostFuncs},
	})
	require.NoError(t, err)

	m, err := store.Instantiate(mod)
	require.NoError(t, err)

	t.Run("host function with f32 param", func(t *testing.T) {
		fn, ok := m.GetFunctionF32Return("call->test.identity_f32")
		require.True(t, ok)

		f32 := float32(math.MaxFloat32)
		result, err := fn(ctx, uint64(math.Float32bits(f32))) // float bits are a uint32 value, call requires uint64
		require.NoError(t, err)
		require.Equal(t, f32, result)
	})

	t.Run("host function with f64 param", func(t *testing.T) {
		fn, ok := m.GetFunctionF64Return("call->test.identity_f64")
		require.True(t, ok)

		f64 := math.MaxFloat64
		result, err := fn(ctx, math.Float64bits(f64))
		require.NoError(t, err)
		require.Equal(t, f64, result)
	})
}
