package vs

import (
	"context"
	"fmt"
	"os"
	"sort"
	"testing"
	"text/tabwriter"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

// ensureJITFastest is overridable via ldflags. Ex.
//	-ldflags '-X github.com/tetratelabs/wazero/internal/integration_test/vs.ensureJITFastest=true'
var ensureJITFastest = "false"

const jitRuntime = "wazero-jit"

var jitFastestBench = func(rt runtime) func(b *testing.B) {
	return func(b *testing.B) {
		benchmarkFn(rt, facConfig, facCall)(b)
	}
}

var runtimes = map[string]func() runtime{
	"wazero-interpreter": newWazeroInterpreterRuntime,
	jitRuntime:           newWazeroJITRuntime,
}

// TestFac_JIT_Fastest ensures that JIT is the fastest engine for function invocations.
// This is disabled by default, and can be run with -ldflags '-X github.com/tetratelabs/wazero/vs.ensureJITFastest=true'.
func TestFac_JIT_Fastest(t *testing.T) {
	if ensureJITFastest != "true" {
		t.Skip()
	}

	type benchResult struct {
		name string
		nsOp float64
	}
	results := make([]benchResult, 0, len(runtimes))

	for name, rtFn := range runtimes {
		result := testing.Benchmark(jitFastestBench(rtFn()))
		// https://github.com/golang/go/blob/fd09e88722e0af150bf8960e95e8da500ad91001/src/testing/benchmark.go#L428-L432
		nsOp := float64(result.T.Nanoseconds()) / float64(result.N)
		results = append(results, benchResult{name: name, nsOp: nsOp})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].nsOp < results[j].nsOp
	})

	// Print results before deciding if this failed
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	for _, result := range results {
		w.Write([]byte(fmt.Sprintf("%s\t%.2f\tns/call\n", result.name, result.nsOp)))
	}
	w.Flush()

	// Fail if jit wasn't fastest!
	require.Equal(t, jitRuntime, results[0].name, "%s is faster than %s. "+
		"Run BenchmarkFac_Call with ensureJITFastest=false instead to see the detailed result",
		results[0].name, jitRuntime)
}

func benchmarkCompile(b *testing.B, rtCfg *runtimeConfig) {
	for name, rtFn := range runtimes {
		rt := rtFn()
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if err := rt.Compile(testCtx, rtCfg); err != nil {
					b.Fatal(err)
				}
				if err := rt.Close(testCtx); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func benchmarkInstantiate(b *testing.B, rtCfg *runtimeConfig) {
	for name, rtFn := range runtimes {
		rt := rtFn()
		b.Run(name, func(b *testing.B) {
			// Compile outside the benchmark loop
			if err := rt.Compile(testCtx, rtCfg); err != nil {
				b.Fatal(err)
			}
			defer rt.Close(testCtx)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				mod, err := rt.Instantiate(testCtx, rtCfg)
				if err != nil {
					b.Fatal(err)
				}
				err = mod.Close(testCtx)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func benchmarkCall(b *testing.B, rtCfg *runtimeConfig, call func(module) (uint64, error)) {
	if ensureJITFastest == "true" {
		// If ensureJITFastest == "true", the benchmark for invocation will be run by
		// TestFac_JIT_Fastest so skip here.
		b.Skip()
	}
	for name, rtFn := range runtimes {
		rt := rtFn()
		b.Run(name, benchmarkFn(rt, rtCfg, call))
	}
}

func benchmarkFn(rt runtime, rtCfg *runtimeConfig, call func(module) (uint64, error)) func(b *testing.B) {
	return func(b *testing.B) {
		// Initialize outside the benchmark loop
		if err := rt.Compile(testCtx, rtCfg); err != nil {
			b.Fatal(err)
		}
		defer rt.Close(testCtx)
		mod, err := rt.Instantiate(testCtx, rtCfg)
		if err != nil {
			b.Fatal(err)
		}
		defer mod.Close(testCtx)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := call(mod); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func testCall(t *testing.T, rtCfg *runtimeConfig, call func(*testing.T, module)) {
	for name, rtFn := range runtimes {
		rt := rtFn()
		t.Run(name, testCallFn(rt, rtCfg, call))
	}
}

func testCallFn(rt runtime, rtCfg *runtimeConfig, testCall func(*testing.T, module)) func(t *testing.T) {
	return func(t *testing.T) {
		err := rt.Compile(testCtx, rtCfg)
		require.NoError(t, err)
		defer rt.Close(testCtx)

		// Ensure the module can be re-instantiated times, even if not all runtimes allow renaming.
		for i := 0; i < 10; i++ {
			m, err := rt.Instantiate(testCtx, rtCfg)
			require.NoError(t, err)

			// Large loop in test is only to show the function is stable (ex doesn't leak or crash on Nth use).
			for j := 0; j < 10000; j++ {
				testCall(t, m)
			}

			require.NoError(t, m.Close(testCtx))
		}
	}
}
