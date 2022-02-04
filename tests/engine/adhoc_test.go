package adhoc

import (
	_ "embed"
	"reflect"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/binary"
	"github.com/tetratelabs/wazero/wasm/interpreter"
	"github.com/tetratelabs/wazero/wasm/jit"
	"github.com/tetratelabs/wazero/wasm/text"
)

func TestJIT(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip()
	}
	runTests(t, jit.NewEngine)
}

func TestInterpreter(t *testing.T) {
	runTests(t, interpreter.NewEngine)
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

func runTests(t *testing.T, newEngine func() wasm.Engine) {
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
}

func fibonacci(t *testing.T, newEngine func() wasm.Engine) {
	mod, err := binary.DecodeModule(fibWasm)
	require.NoError(t, err)

	// We execute 1000 times in order to ensure the JIT engine is stable under high concurrency
	// and we have no conflict with Go's runtime.
	const goroutines = 1000
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			store := wasm.NewStore(newEngine())
			require.NoError(t, err)
			err = store.Instantiate(mod, "test")
			require.NoError(t, err)
			out, _, err := store.CallFunction("test", "fib", 20)
			require.NoError(t, err)
			require.Equal(t, uint64(10946), out[0])
		}()
	}
	wg.Wait()
}

func fac(t *testing.T, newEngine func() wasm.Engine) {
	mod, err := binary.DecodeModule(facWasm)
	require.NoError(t, err)
	store := wasm.NewStore(newEngine())
	require.NoError(t, err)
	err = store.Instantiate(mod, "test")
	require.NoError(t, err)
	for _, name := range []string{
		"fac-rec",
		"fac-iter",
		"fac-rec-named",
		"fac-iter-named",
		"fac-opt",
	} {
		name := name
		t.Run(name, func(t *testing.T) {
			out, _, err := store.CallFunction("test", name, 25)
			require.NoError(t, err)
			require.Equal(t, uint64(7034535277573963776), out[0])
		})
	}

	_, _, err = store.CallFunction("test", "fac-rec", 1073741824)
	require.ErrorIs(t, err, wasm.ErrRuntimeCallStackOverflow)
}

func unreachable(t *testing.T, newEngine func() wasm.Engine) {
	mod, err := binary.DecodeModule(unreachableWasm)
	require.NoError(t, err)
	store := wasm.NewStore(newEngine())
	require.NoError(t, err)

	const moduleName = "test"

	callUnreachable := func(ctx *wasm.HostFunctionCallContext) {
		_, _, err := store.CallFunction(moduleName, "unreachable_func")
		require.NoError(t, err)
	}
	err = store.AddHostFunction("host", "cause_unreachable", reflect.ValueOf(callUnreachable))
	require.NoError(t, err)

	err = store.Instantiate(mod, moduleName)
	require.NoError(t, err)

	_, _, err = store.CallFunction(moduleName, "main")
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

func memory(t *testing.T, newEngine func() wasm.Engine) {
	mod, err := binary.DecodeModule(memoryWasm)
	require.NoError(t, err)
	store := wasm.NewStore(newEngine())
	require.NoError(t, err)
	err = store.Instantiate(mod, "test")
	require.NoError(t, err)
	// First, we have zero-length memory instance.
	out, _, err := store.CallFunction("test", "size")
	require.NoError(t, err)
	require.Equal(t, uint64(0), out[0])
	// Then grow the memory.
	const newPages uint64 = 10
	out, _, err = store.CallFunction("test", "grow", newPages)
	require.NoError(t, err)
	// Grow returns the previous number of memory pages, namely zero.
	require.Equal(t, uint64(0), out[0])
	// Now size should return the new pages -- 10.
	out, _, err = store.CallFunction("test", "size")
	require.NoError(t, err)
	require.Equal(t, newPages, out[0])
	// Growing memory with zero pages is valid but should be noop.
	out, _, err = store.CallFunction("test", "grow", 0)
	require.NoError(t, err)
	require.Equal(t, newPages, out[0])
}

func recursiveEntry(t *testing.T, newEngine func() wasm.Engine) {
	mod, err := binary.DecodeModule(recursiveWasm)
	require.NoError(t, err)

	store := wasm.NewStore(newEngine())

	hostfunc := func(ctx *wasm.HostFunctionCallContext) {
		_, _, err := store.CallFunction("test", "called_by_host_func")
		require.NoError(t, err)
	}
	err = store.AddHostFunction("env", "host_func", reflect.ValueOf(hostfunc))
	require.NoError(t, err)

	err = store.Instantiate(mod, "test")
	require.NoError(t, err)

	_, _, err = store.CallFunction("test", "main", uint64(1))
	require.NoError(t, err)
}

func importedAndExportedFunc(t *testing.T, newEngine func() wasm.Engine) {
	// Test that the engine can call "imported-and-then-exported-back" function correctly
	mod, err := text.DecodeModule([]byte(`(module
		;; arbitrary function with params
		(import "env" "add_int" (func $add_int (param i32 i32) (result i32)))
		;; add_int is imported from the environment, but it's also exported back to the environment
		(export "add_int" (func $add_int))
		)`))
	require.NoError(t, err)

	store := wasm.NewStore(newEngine())

	addInt := func(ctx *wasm.HostFunctionCallContext, x int32, y int32) int32 {
		return x + y
	}
	err = store.AddHostFunction("env", "add_int", reflect.ValueOf(addInt))
	require.NoError(t, err)

	err = store.Instantiate(mod, "test")
	require.NoError(t, err)

	// We should be able to call the exported add_int and it should add two ints.
	results, _, err := store.CallFunction("test", "add_int", uint64(12), uint64(30))
	require.NoError(t, err)
	require.Equal(t, uint64(42), results[0])
}
