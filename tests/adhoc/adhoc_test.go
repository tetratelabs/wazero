package adhoc

import (
	"os"
	"reflect"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/binary"
	"github.com/tetratelabs/wazero/wasm/interpreter"
	"github.com/tetratelabs/wazero/wasm/jit"
)

func TestJIT(t *testing.T) {
	runTests(t, jit.NewEngine)
}

func TestInterpreter(t *testing.T) {
	runTests(t, interpreter.NewEngine)
}

func runTests(t *testing.T, newEngine func() wasm.Engine) {
	fibonacci(t, newEngine)
	fac(t, newEngine)
	unreachable(t, newEngine)
	memory(t, newEngine)
	recursiveEntry(t, newEngine)
}

func fibonacci(t *testing.T, newEngine func() wasm.Engine) {
	buf, err := os.ReadFile("testdata/fib.wasm")
	require.NoError(t, err)
	mod, err := binary.DecodeModule(buf)
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
	buf, err := os.ReadFile("testdata/fac.wasm")
	require.NoError(t, err)
	mod, err := binary.DecodeModule(buf)
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
	buf, err := os.ReadFile("testdata/unreachable.wasm")
	require.NoError(t, err)
	mod, err := binary.DecodeModule(buf)
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
	buf, err := os.ReadFile("testdata/memory.wasm")
	require.NoError(t, err)
	mod, err := binary.DecodeModule(buf)
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
	buf, err := os.ReadFile("testdata/recursive.wasm")
	require.NoError(t, err)
	mod, err := binary.DecodeModule(buf)
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
