package vs

import (
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

const (
	facArgument = uint64(30)
	facResult   = uint64(9682165104862298112)
)

// facWasm is compiled from testdata/fac.wat
//go:embed testdata/fac.wasm
var facWasm []byte

func facInit(rt runtimeTester) error {
	return rt.Init(testCtx, facWasm, "fac")
}

func facInvoke(rt runtimeTester) (uint64, error) {
	return rt.CallI64_I64(testCtx, "fac", facArgument)
}

func testFacInvoke(rt runtimeTester) func(t *testing.T) {
	return func(t *testing.T) {
		err := facInit(rt)
		require.NoError(t, err)
		defer rt.Close()

		// Large loop in test is only to show the function is stable (ex doesn't leak or crash on Nth use).
		for i := 0; i < 10000; i++ {
			res, err := facInvoke(rt)
			require.NoError(t, err)
			require.Equal(t, facResult, res)
		}
	}
}

func benchFacInvoke(rt runtimeTester) func(b *testing.B) {
	return func(b *testing.B) {
		// Initialize outside the benchmark loop
		if err := facInit(rt); err != nil {
			b.Fatal(err)
		}
		defer rt.Close()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := facInvoke(rt); err != nil {
				b.Fatal(err)
			}
		}
	}
}

// TestFac ensures that the code in BenchmarkFac works as expected.
func TestFac(t *testing.T) {
	for name, rtFn := range runtimeTesters {
		t.Run(name, testFacInvoke(rtFn()))
	}
}

// BenchmarkFac_Init tracks the time spent readying a function for use
func BenchmarkFac_Init(b *testing.B) {
	for name, rtFn := range runtimeTesters {
		rt := rtFn()
		b.Run(name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := facInit(rt); err != nil {
					b.Fatal(err)
				} else {
					rt.Close()
				}
			}
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
	for name, rtFn := range runtimeTesters {
		b.Run(name, benchFacInvoke(rtFn()))
	}
}
