package adhoc

import (
	"bytes"
	"context"
	_ "embed"
	"math"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"text/template"

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
	t.Run("host function with float32 type", func(t *testing.T) {
		hostFuncWithFloatParam(t, newEngine)
	})
}

func fibonacci(t *testing.T, newEngine func() wasm.Engine) {
	ctx := context.Background()
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
			out, _, err := store.CallFunction(ctx, "test", "fib", 20)
			require.NoError(t, err)
			require.Equal(t, uint64(10946), out[0])
		}()
	}
	wg.Wait()
}

func fac(t *testing.T, newEngine func() wasm.Engine) {
	ctx := context.Background()
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
			out, _, err := store.CallFunction(ctx, "test", name, 25)
			require.NoError(t, err)
			require.Equal(t, uint64(7034535277573963776), out[0])
		})
	}

	_, _, err = store.CallFunction(ctx, "test", "fac-rec", 1073741824)
	require.ErrorIs(t, err, wasm.ErrRuntimeCallStackOverflow)
}

func unreachable(t *testing.T, newEngine func() wasm.Engine) {
	ctx := context.Background()
	mod, err := binary.DecodeModule(unreachableWasm)
	require.NoError(t, err)
	store := wasm.NewStore(newEngine())
	require.NoError(t, err)

	const moduleName = "test"

	callUnreachable := func(ctx *wasm.HostFunctionCallContext) {
		_, _, err := store.CallFunction(ctx.Context(), moduleName, "unreachable_func")
		require.NoError(t, err)
	}
	err = store.AddHostFunction("host", "cause_unreachable", reflect.ValueOf(callUnreachable))
	require.NoError(t, err)

	err = store.Instantiate(mod, moduleName)
	require.NoError(t, err)

	_, _, err = store.CallFunction(ctx, moduleName, "main")
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
	ctx := context.Background()
	mod, err := binary.DecodeModule(memoryWasm)
	require.NoError(t, err)
	store := wasm.NewStore(newEngine())
	require.NoError(t, err)
	err = store.Instantiate(mod, "test")
	require.NoError(t, err)
	// First, we have zero-length memory instance.
	out, _, err := store.CallFunction(ctx, "test", "size")
	require.NoError(t, err)
	require.Equal(t, uint64(0), out[0])
	// Then grow the memory.
	const newPages uint64 = 10
	out, _, err = store.CallFunction(ctx, "test", "grow", newPages)
	require.NoError(t, err)
	// Grow returns the previous number of memory pages, namely zero.
	require.Equal(t, uint64(0), out[0])
	// Now size should return the new pages -- 10.
	out, _, err = store.CallFunction(ctx, "test", "size")
	require.NoError(t, err)
	require.Equal(t, newPages, out[0])
	// Growing memory with zero pages is valid but should be noop.
	out, _, err = store.CallFunction(ctx, "test", "grow", 0)
	require.NoError(t, err)
	require.Equal(t, newPages, out[0])
}

func recursiveEntry(t *testing.T, newEngine func() wasm.Engine) {
	ctx := context.Background()
	mod, err := binary.DecodeModule(recursiveWasm)
	require.NoError(t, err)

	store := wasm.NewStore(newEngine())

	hostfunc := func(ctx *wasm.HostFunctionCallContext) {
		_, _, err := store.CallFunction(ctx.Context(), "test", "called_by_host_func")
		require.NoError(t, err)
	}
	err = store.AddHostFunction("env", "host_func", reflect.ValueOf(hostfunc))
	require.NoError(t, err)

	err = store.Instantiate(mod, "test")
	require.NoError(t, err)

	_, _, err = store.CallFunction(ctx, "test", "main", uint64(1))
	require.NoError(t, err)
}

// importedAndExportedFunc fails if the engine cannot call an "imported-and-then-exported-back" function
func importedAndExportedFunc(t *testing.T, newEngine func() wasm.Engine) {
	ctx := context.Background()
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
	results, _, err := store.CallFunction(ctx, "test", "add_int", uint64(12), uint64(30))
	require.NoError(t, err)
	require.Equal(t, uint64(42), results[0])
}

//  hostFuncWithFloatParam fails if a float parameter corrupts a host function value
func hostFuncWithFloatParam(t *testing.T, newEngine func() wasm.Engine) {
	ctx := context.Background()
	watTemplate, err := template.New("").Parse(`(module
		;; '{{ . }}' will be replaced by the type name, f32 or f64.

		;; 'identity_float' just returns the given float value as is.
		(import "test" "identity_float" (func $test.identity_float (param {{ . }}) (result {{ . }})))

		;; 'call->test.identity_float' proxies 'test.identity_float' via call in order to test floats aren't corrupted 
		;; when the float values are passed through OpCodeCall
		(func $call->test.identity_float (param {{ . }}) (result {{ . }})
			local.get 0
			call $test.identity_float
		)
		(export "call->test.identity_float" (func $call->test.identity_float))
		)`)
	require.NoError(t, err)

	tests := []struct {
		testName          string
		floatType         byte          // wasm.ValueTypeF32 or wasm.ValueTypeF64
		identityFloatFunc reflect.Value // imported as 'identity_float' to the wasm module
		floatParam        uint64        // passed to identityFloatFunc as a parameter
		expectedFloatVal  float64
	}{
		{
			testName:  "host function with f32 param",
			floatType: wasm.ValueTypeF32,
			identityFloatFunc: reflect.ValueOf(func(ctx *wasm.HostFunctionCallContext, value float32) float32 {
				return value
			}),
			floatParam:       uint64(math.Float32bits(math.MaxFloat32)), // float bits as a uint32 value, but casted to uint64 to be passed to CallFunction
			expectedFloatVal: float64(math.MaxFloat32),                  // arbitrary f32 value
		},
		{
			testName:  "host function with f64 param",
			floatType: wasm.ValueTypeF64,
			identityFloatFunc: reflect.ValueOf(func(ctx *wasm.HostFunctionCallContext, value float64) float64 {
				return value
			}),
			floatParam:       math.Float64bits(math.MaxFloat64),
			expectedFloatVal: float64(math.MaxFloat64), // arbitrary f64 value that doesn' fit in f32
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.testName, func(t *testing.T) {
			// Build and instantiate the wat for `f32` or `f64`
			var wat bytes.Buffer
			err = watTemplate.Execute(&wat, wasm.ValueTypeName(tc.floatType))
			require.NoError(t, err)

			mod, err := text.DecodeModule(wat.Bytes())
			require.NoError(t, err)

			store := wasm.NewStore(newEngine())

			err = store.AddHostFunction("test", "identity_float", tc.identityFloatFunc)
			require.NoError(t, err)

			err = store.Instantiate(mod, "mod")
			require.NoError(t, err)

			// This call should return the `expectedFloatVal`.
			// That ensures that the float32 values are not corrupted when they are routed through OpCodeCall.
			results, resultTypes, err := store.CallFunction(ctx, "mod", "call->test.identity_float", tc.floatParam)
			require.NoError(t, err)
			require.Len(t, results, len(resultTypes))
			require.Equal(t, tc.floatType, resultTypes[0])
			// Note that the both f32 and f64 result value are expressed as the float64 bits value in wazero.
			require.Equal(t, tc.expectedFloatVal, math.Float64frombits(results[0]))
		})
	}
}
