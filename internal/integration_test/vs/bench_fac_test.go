//go:build amd64 && cgo && !windows

// Wasmtime can only be used in amd64 with CGO
// Wasmer doesn't link on Windows
package vs

import (
	"context"
	_ "embed"
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

// ensureJITFastest is overridable via ldflags. Ex.
//	-ldflags '-X github.com/tetratelabs/wazero/vs.ensureJITFastest=true'
var ensureJITFastest = "false"

// facWasm is compiled from testdata/fac.wat
//go:embed testdata/fac.wasm
var facWasm []byte

// TestFac ensures that the code in BenchmarkFac works as expected.
func TestFac(t *testing.T) {
	const in = 30
	expValue := uint64(0x865df5dd54000000)

	t.Run("Interpreter", func(t *testing.T) {
		config := wazero.NewRuntimeConfigInterpreter().WithFinishedFeatures()
		rt := newWazeroTester(config)
		testRtFac(t, rt, in, expValue)
	})

	t.Run("JIT", func(t *testing.T) {
		config := wazero.NewRuntimeConfigJIT().WithFinishedFeatures()
		rt := newWazeroTester(config)
		testRtFac(t, rt, in, expValue)
	})

	t.Run("wasmer-go", func(t *testing.T) {
		rt := newWasmerTester()
		testRtFac(t, rt, in, expValue)
	})

	t.Run("wasmtime-go", func(t *testing.T) {
		rt := newWasmtimeTester()
		testRtFac(t, rt, in, expValue)
	})

	t.Run("go-wasm3", func(t *testing.T) {
		rt := newWasm3Tester()
		testRtFac(t, rt, in, expValue)
	})
}

func testRtFac(t *testing.T, rt runtimeTester, in int, expValue uint64) {
	err := rt.Init(testCtx, facWasm, "fac")
	require.NoError(t, err)
	defer rt.Close()

	for i := 0; i < 10000; i++ {
		res, err := rt.Call(testCtx, "fac", uint64(in))
		require.NoError(t, err)
		require.Equal(t, expValue, res)
	}
}

// BenchmarkFac_Init tracks the time spent readying a function for use
func BenchmarkFac_Init(b *testing.B) {
	b.Run("Interpreter", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rt := newWazeroTester(wazero.NewRuntimeConfigInterpreter().WithFinishedFeatures())
			if err := rtFacInit(rt); err != nil {
				b.Fatal(err)
			} else {
				rt.Close()
			}
		}
	})

	b.Run("JIT", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rt := newWazeroTester(wazero.NewRuntimeConfigJIT().WithFinishedFeatures())
			if err := rtFacInit(rt); err != nil {
				b.Fatal(err)
			} else {
				rt.Close()
			}
		}
	})

	b.Run("wasmer-go", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rt := newWasmerTester()
			if err := rtFacInit(rt); err != nil {
				b.Fatal(err)
			} else {
				rt.Close()
			}
		}
	})

	b.Run("wasmtime-go", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rt := newWasmtimeTester()
			if err := rtFacInit(rt); err != nil {
				b.Fatal(err)
			} else {
				rt.Close()
			}
		}
	})

	b.Run("go-wasm3", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rt := newWasm3Tester()
			if err := rtFacInit(rt); err != nil {
				b.Fatal(err)
			} else {
				rt.Close()
			}
		}
	})
}

var facArgumentU64 = uint64(30)
var facArgumentI64 = int64(facArgumentU64)

// TestFac_JIT_Fastest ensures that JIT is the fastest engine for function invocations.
// This is disabled by default, and can be run with -ldflags '-X github.com/tetratelabs/wazero/vs.ensureJITFastest=true'.
func TestFac_JIT_Fastest(t *testing.T) {
	if ensureJITFastest != "true" {
		t.Skip()
	}

	jitResult := testing.Benchmark(jitFacInvoke)

	cases := []struct {
		runtimeName string
		result      testing.BenchmarkResult
	}{
		{
			runtimeName: "interpreter",
			result:      testing.Benchmark(interpreterFacInvoke),
		},
		{
			runtimeName: "wasmer-go",
			result:      testing.Benchmark(wasmerFacInvoke),
		},
		{
			runtimeName: "wasmtime-go",
			result:      testing.Benchmark(wasmtimeFacInvoke),
		},
		{
			runtimeName: "go-wasm3",
			result:      testing.Benchmark(wasm3FacInvoke),
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
			require.True(t, jitNanoPerOp < nanoPerOp, "jitNanoPerOp(%f) is not less than nanoPerOp(%f). JIT engine must be faster than %s. "+
				"Run BenchmarkFac_Invoke with ensureJITFastest=false instead to see the detailed result",
				tc.runtimeName, jitNanoPerOp, nanoPerOp)
		})
	}
}

// BenchmarkFac_Invoke benchmarks the time spent invoking a factorial calculation.
func BenchmarkFac_Invoke(b *testing.B) {
	if ensureJITFastest == "true" {
		// If ensureJITFastest == "true", the benchmark for invocation will be run by
		// TestFac_JIT_Fastest so skip here.
		b.Skip()
	}
	b.Run("Interpreter", interpreterFacInvoke)
	b.Run("JIT", jitFacInvoke)
	b.Run("wasmer-go", wasmerFacInvoke)
	b.Run("wasmtime-go", wasmtimeFacInvoke)
	b.Run("go-wasm3", wasm3FacInvoke)
}

func interpreterFacInvoke(b *testing.B) {
	wazeroFacInvoke(b, wazero.NewRuntimeConfigInterpreter().WithFinishedFeatures())
}

func jitFacInvoke(b *testing.B) {
	wazeroFacInvoke(b, wazero.NewRuntimeConfigJIT().WithFinishedFeatures())
}

func wazeroFacInvoke(b *testing.B, config *wazero.RuntimeConfig) {
	rt := newWazeroTester(config)
	err := rtFacInit(rt)
	rtFacInvoke(b, err, rt)
}

func wasmerFacInvoke(b *testing.B) {
	rt := newWasmerTester()
	err := rtFacInit(rt)
	rtFacInvoke(b, err, rt)
}

func wasmtimeFacInvoke(b *testing.B) {
	rt := newWasmtimeTester()
	err := rtFacInit(rt)
	rtFacInvoke(b, err, rt)
}

func wasm3FacInvoke(b *testing.B) {
	rt := newWasm3Tester()
	err := rtFacInit(rt)
	rtFacInvoke(b, err, rt)
}

func rtFacInvoke(b *testing.B, err error, rt runtimeTester) {
	if err != nil {
		b.Fatal(err)
	}
	defer rt.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err = rt.Call(testCtx, "fac", facArgumentU64); err != nil {
			b.Fatal(err)
		}
	}
}

func rtFacInit(rt runtimeTester) error {
	return rt.Init(testCtx, facWasm, "fac")
}
