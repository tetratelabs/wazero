//go:build amd64 && cgo && !windows

// Wasmtime can only be used in amd64 with CGO
// Wasmer doesn't link on Windows
package vs

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"testing"

	"github.com/bytecodealliance/wasmtime-go"
	"github.com/stretchr/testify/require"
	"github.com/wasmerio/wasmer-go/wasmer"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/wasm"
)

// ensureJITFastest is overridable via ldflags. Ex.
//	-ldflags '-X github.com/tetratelabs/wazero/vs.ensureJITFastest=true'
var ensureJITFastest string = "false"

// facWasm is compiled from testdata/fac.wat
//go:embed testdata/fac.wasm
var facWasm []byte

// TestFacIter ensures that the code in BenchmarkFacIter works as expected.
func TestFacIter(t *testing.T) {
	ctx := context.Background()
	const in = 30
	expValue := uint64(0x865df5dd54000000)

	t.Run("Interpreter", func(t *testing.T) {
		fn, err := newWazeroFacIterBench(wazero.NewRuntimeConfigInterpreter())
		require.NoError(t, err)

		for i := 0; i < 10000; i++ {
			res, err := fn.Call(ctx, in)
			require.NoError(t, err)
			require.Equal(t, expValue, res[0])
		}
	})

	t.Run("JIT", func(t *testing.T) {
		fn, err := newWazeroFacIterBench(wazero.NewRuntimeConfigJIT())
		require.NoError(t, err)

		for i := 0; i < 10000; i++ {
			res, err := fn.Call(ctx, in)
			require.NoError(t, err)
			require.Equal(t, expValue, res[0])
		}
	})

	t.Run("wasmer-go", func(t *testing.T) {
		store, instance, fn, err := newWasmerForFacIterBench()
		require.NoError(t, err)
		defer store.Close()
		defer instance.Close()

		for i := 0; i < 10000; i++ {
			res, err := fn(in)
			require.NoError(t, err)
			require.Equal(t, int64(expValue), res)
		}
	})

	t.Run("wasmtime-go", func(t *testing.T) {
		store, run, err := newWasmtimeForFacIterBench()
		require.NoError(t, err)
		for i := 0; i < 10000; i++ {
			res, err := run.Call(store, in)
			if err != nil {
				panic(err)
			}
			require.Equal(t, int64(expValue), res)
		}
	})
}

// BenchmarkFacIter_Init tracks the time spent readying a function for use
func BenchmarkFacIter_Init(b *testing.B) {
	b.Run("Interpreter", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := newWazeroFacIterBench(wazero.NewRuntimeConfigInterpreter()); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("JIT", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := newWazeroFacIterBench(wazero.NewRuntimeConfigJIT()); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("wasmer-go", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			store, instance, _, err := newWasmerForFacIterBench()
			if err != nil {
				b.Fatal(err)
			}
			store.Close()
			instance.Close()
		}
	})

	b.Run("wasmtime-go", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, _, err := newWasmtimeForFacIterBench(); err != nil {
				b.Fatal(err)
			}
		}
	})
}

var ctx = context.Background()
var facIterArgumentU64 uint64 = 30
var facIterArgumentI64 int64 = int64(facIterArgumentU64)

// TestFacIter_JIT_Fastest ensures that JIT is the fastest engine for function invocations.
// This is disabled by default, and can be run with -ldflags '-X github.com/tetratelabs/wazero/vs.ensureJITFastest=true'.
func TestFacIter_JIT_Fastest(t *testing.T) {
	if ensureJITFastest != "true" {
		t.Skip()
	}

	jitResult := testing.Benchmark(jitFacIterInvoke)

	cases := []struct {
		runtimeName string
		result      testing.BenchmarkResult
	}{
		{
			runtimeName: "interpreter",
			result:      testing.Benchmark(interpreterFacIterInvoke),
		},
		{
			runtimeName: "wasmer-go",
			result:      testing.Benchmark(wasmerGoFacIterInvoke),
		},
		{
			runtimeName: "wasmtime-go",
			result:      testing.Benchmark(wasmtimeGoFacIterInvoke),
		},
	}

	// Print results before running each subtest.
	fmt.Println("JIT", jitResult)
	for _, tc := range cases {
		fmt.Println(tc.runtimeName, tc.result)
	}

	jitNanoPerOp := float64(jitResult.T.Nanoseconds()) / float64(jitResult.N)
	for _, tc := range cases {
		tc := tc
		t.Run(tc.runtimeName, func(t *testing.T) {
			// https://github.com/golang/go/blob/fd09e88722e0af150bf8960e95e8da500ad91001/src/testing/benchmark.go#L428-L432
			nanoPerOp := float64(tc.result.T.Nanoseconds()) / float64(tc.result.N)
			msg := fmt.Sprintf("JIT engine must be faster than %s. "+
				"Run BenchmarkFacIter_Invoke with ensureJITFastest=false instead to see the detailed result",
				tc.runtimeName)
			require.Lessf(t, jitNanoPerOp, nanoPerOp, msg)
		})
	}
}

// BenchmarkFacIter_Invoke benchmarks the time spent invoking a factorial calculation.
func BenchmarkFacIter_Invoke(b *testing.B) {
	if ensureJITFastest == "true" {
		// If ensureJITFastest == "true", the benchmark for invocation will be run by
		// TestFacIter_JIT_Fastest so skip here.
		b.Skip()
	}
	b.Run("Interpreter", interpreterFacIterInvoke)
	b.Run("JIT", jitFacIterInvoke)
	b.Run("wasmer-go", wasmerGoFacIterInvoke)
	b.Run("wasmtime-go", wasmtimeGoFacIterInvoke)
}

func interpreterFacIterInvoke(b *testing.B) {
	fn, err := newWazeroFacIterBench(wazero.NewRuntimeConfigInterpreter())
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err = fn.Call(ctx, facIterArgumentU64); err != nil {
			b.Fatal(err)
		}
	}
}

func jitFacIterInvoke(b *testing.B) {
	fn, err := newWazeroFacIterBench(wazero.NewRuntimeConfigJIT())
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err = fn.Call(ctx, facIterArgumentU64); err != nil {
			b.Fatal(err)
		}
	}
}

func wasmerGoFacIterInvoke(b *testing.B) {
	store, instance, fn, err := newWasmerForFacIterBench()
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	defer instance.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err = fn(facIterArgumentI64); err != nil {
			b.Fatal(err)
		}
	}
}

func wasmtimeGoFacIterInvoke(b *testing.B) {
	store, run, err := newWasmtimeForFacIterBench()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err = run.Call(store, facIterArgumentI64); err != nil {
			b.Fatal(err)
		}
	}
}

func newWazeroFacIterBench(engine *wazero.RuntimeConfig) (wasm.Function, error) {
	r := wazero.NewRuntimeWithConfig(engine)

	m, err := r.NewModuleFromSource(facWasm)
	if err != nil {
		return nil, err
	}

	return m.ExportedFunction("fac-iter"), nil
}

// newWasmerForFacIterBench returns the store and instance that scope the factorial function.
// Note: these should be closed
func newWasmerForFacIterBench() (*wasmer.Store, *wasmer.Instance, wasmer.NativeFunction, error) {
	store := wasmer.NewStore(wasmer.NewEngine())
	importObject := wasmer.NewImportObject()
	module, err := wasmer.NewModule(store, facWasm)
	if err != nil {
		return nil, nil, nil, err
	}
	instance, err := wasmer.NewInstance(module, importObject)
	if err != nil {
		return nil, nil, nil, err
	}
	f, err := instance.Exports.GetFunction("fac-iter")
	if err != nil {
		return nil, nil, nil, err
	}
	if f == nil {
		return nil, nil, nil, errors.New("not a function")
	}
	return store, instance, f, nil
}

func newWasmtimeForFacIterBench() (*wasmtime.Store, *wasmtime.Func, error) {
	store := wasmtime.NewStore(wasmtime.NewEngine())
	module, err := wasmtime.NewModule(store.Engine, facWasm)
	if err != nil {
		return nil, nil, err
	}

	instance, err := wasmtime.NewInstance(store, module, nil)
	if err != nil {
		return nil, nil, err
	}

	run := instance.GetFunc(store, "fac-iter")
	if run == nil {
		return nil, nil, errors.New("not a function")
	}
	return store, run, nil
}
