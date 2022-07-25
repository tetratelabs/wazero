package vs

import (
	"context"
	"fmt"
	"os"
	"path"
	"runtime"
	"sort"
	"testing"
	"text/tabwriter"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

// ensureCompilerFastest is overridable via ldflags. Ex.
//
//	-ldflags '-X github.com/tetratelabs/wazero/internal/integration_test/vs.ensureCompilerFastest=true'
var ensureCompilerFastest = "false"

const compilerRuntime = "wazero-compiler"

// runTestBenchmark_Call_CompilerFastest ensures that Compiler is the fastest engine for function invocations.
// This is disabled by default, and can be run with -ldflags '-X github.com/tetratelabs/wazero/vs.ensureCompilerFastest=true'.
func runTestBenchmark_Call_CompilerFastest(t *testing.T, rtCfg *RuntimeConfig, name string, call func(Module) error, vsRuntime Runtime) {
	if ensureCompilerFastest != "true" {
		t.Skip()
	}

	type benchResult struct {
		name string
		nsOp float64
	}

	results := make([]benchResult, 0, 2)
	// Add the result for Compiler
	compilerNsOp := runCallBenchmark(NewWazeroCompilerRuntime(), rtCfg, call)
	results = append(results, benchResult{name: compilerRuntime, nsOp: compilerNsOp})

	// Add a result for the runtime we're comparing against
	vsNsOp := runCallBenchmark(vsRuntime, rtCfg, call)
	results = append(results, benchResult{name: vsRuntime.Name(), nsOp: vsNsOp})

	sort.Slice(results, func(i, j int) bool {
		return results[i].nsOp < results[j].nsOp
	})

	// Print results before deciding if this failed
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	_, _ = fmt.Fprintf(w, "Benchmark%s/Call-16\n", name)
	for _, result := range results {
		_, _ = fmt.Fprintf(w, "%s\t%.2f\tns/op\n", result.name, result.nsOp)
	}
	_ = w.Flush()

	// Fail if compiler wasn't fastest!
	require.Equal(t, compilerRuntime, results[0].name, "%s is faster than %s. "+
		"Run with ensureCompilerFastest=false instead to see the detailed result",
		results[0].name, compilerRuntime)
}

func runCallBenchmark(rt Runtime, rtCfg *RuntimeConfig, call func(Module) error) float64 {
	result := testing.Benchmark(func(b *testing.B) {
		benchmarkCall(b, rt, rtCfg, call)
	})
	// https://github.com/golang/go/blob/fd09e88722e0af150bf8960e95e8da500ad91001/src/testing/benchmark.go#L428-L432
	nsOp := float64(result.T.Nanoseconds()) / float64(result.N)
	return nsOp
}

func benchmark(b *testing.B, runtime func() Runtime, rtCfg *RuntimeConfig, call func(Module) error) {
	rt := runtime()
	b.Run("Compile", func(b *testing.B) {
		benchmarkCompile(b, rt, rtCfg)
	})
	b.Run("Instantiate", func(b *testing.B) {
		benchmarkInstantiate(b, rt, rtCfg)
	})

	// Don't burn CPU when this is already going to be called in runTestBenchmark_Call_CompilerFastest
	if ensureCompilerFastest != "true" || rt.Name() == compilerRuntime {
		b.Run("Call", func(b *testing.B) {
			benchmarkCall(b, rt, rtCfg, call)
		})
	}
}

func benchmarkCompile(b *testing.B, rt Runtime, rtCfg *RuntimeConfig) {
	for i := 0; i < b.N; i++ {
		if err := rt.Compile(testCtx, rtCfg); err != nil {
			b.Fatal(err)
		}
		if err := rt.Close(testCtx); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkInstantiate(b *testing.B, rt Runtime, rtCfg *RuntimeConfig) {
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
}

func benchmarkCall(b *testing.B, rt Runtime, rtCfg *RuntimeConfig, call func(Module) error) {
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
		if err := call(mod); err != nil {
			b.Fatal(err)
		}
	}
}

func testCall(t *testing.T, runtime func() Runtime, rtCfg *RuntimeConfig, testCall func(*testing.T, Module, int, int)) {
	rt := runtime()
	err := rt.Compile(testCtx, rtCfg)
	require.NoError(t, err)
	defer rt.Close(testCtx)

	// Ensure the module can be re-instantiated times, even if not all runtimes allow renaming.
	for i := 0; i < 10; i++ {
		m, err := rt.Instantiate(testCtx, rtCfg)
		require.NoError(t, err)

		// Large loop in test is only to show the function is stable (ex doesn't leak or crash on Nth use).
		for j := 0; j < 1000; j++ {
			testCall(t, m, i, j)
		}

		require.NoError(t, m.Close(testCtx))
	}
}

func readRelativeFile(relativePath string) []byte {
	// We can't resolve relative paths as init() is called from each of its subdirs
	_, source, _, _ := runtime.Caller(1) // 1 as this utility is in a different source than the caller.
	realPath := path.Join(path.Dir(source), relativePath)
	bytes, err := os.ReadFile(realPath)
	if err != nil {
		panic(err)
	}
	return bytes
}
