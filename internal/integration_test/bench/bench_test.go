package bench

import (
	"context"
	"crypto/rand"
	_ "embed"
	"fmt"
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/platform"
)

type arbitrary struct{}

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), arbitrary{}, "arbitrary")

// caseWasm was compiled from TinyGo testdata/case.go
//
//go:embed testdata/case.wasm
var caseWasm []byte

func BenchmarkInvocation(b *testing.B) {
	b.Run("interpreter", func(b *testing.B) {
		m := instantiateHostFunctionModuleWithEngine(b, wazero.NewRuntimeConfigInterpreter())
		defer m.Close(testCtx)
		runAllInvocationBenches(b, m)
	})
	if runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" {
		b.Run("compiler", func(b *testing.B) {
			m := instantiateHostFunctionModuleWithEngine(b, wazero.NewRuntimeConfigCompiler())
			defer m.Close(testCtx)
			runAllInvocationBenches(b, m)
		})
	}
}

func BenchmarkInitialization(b *testing.B) {
	b.Run("interpreter", func(b *testing.B) {
		r := createRuntime(b, wazero.NewRuntimeConfigInterpreter())
		runInitializationBench(b, r)
	})

	b.Run("interpreter-multiple", func(b *testing.B) {
		r := createRuntime(b, wazero.NewRuntimeConfigInterpreter())
		runInitializationConcurrentBench(b, r)
	})

	if platform.CompilerSupported() {
		b.Run("compiler", func(b *testing.B) {
			r := createRuntime(b, wazero.NewRuntimeConfigCompiler())
			runInitializationBench(b, r)
		})

		b.Run("compiler-multiple", func(b *testing.B) {
			r := createRuntime(b, wazero.NewRuntimeConfigCompiler())
			runInitializationConcurrentBench(b, r)
		})
	}
}

func BenchmarkCompilation(b *testing.B) {
	if !platform.CompilerSupported() {
		b.Skip()
	}

	// Note: recreate runtime each time in the loop to ensure that
	// recompilation happens if the extern cache is not used.
	b.Run("with extern cache", func(b *testing.B) {
		cache, err := wazero.NewCompilationCacheWithDir(b.TempDir())
		if err != nil {
			b.Fatal(err)
		}
		for i := 0; i < b.N; i++ {
			r := wazero.NewRuntimeWithConfig(context.Background(), wazero.NewRuntimeConfigCompiler().WithCompilationCache(cache))
			runCompilation(b, r)
		}
	})
	b.Run("without extern cache", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			r := wazero.NewRuntimeWithConfig(context.Background(), wazero.NewRuntimeConfigCompiler())
			runCompilation(b, r)
		}
	})
	b.Run("interpreter", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			r := wazero.NewRuntimeWithConfig(context.Background(), wazero.NewRuntimeConfigInterpreter())
			runCompilation(b, r)
		}
	})
}

func runCompilation(b *testing.B, r wazero.Runtime) wazero.CompiledModule {
	compiled, err := r.CompileModule(testCtx, caseWasm)
	if err != nil {
		b.Fatal(err)
	}
	return compiled
}

func runInitializationBench(b *testing.B, r wazero.Runtime) {
	compiled := runCompilation(b, r)
	defer compiled.Close(testCtx)
	// Configure with real sources to avoid performance hit initializing fake ones. These sources are not used
	// in the benchmark.
	config := wazero.NewModuleConfig().WithSysNanotime().WithSysWalltime().WithRandSource(rand.Reader).
		// To measure the pure instantiation time without including calling _start.
		WithStartFunctions()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mod, err := r.InstantiateModule(testCtx, compiled, config)
		if err != nil {
			b.Fatal(err)
		}
		mod.Close(testCtx)
	}
}

func runInitializationConcurrentBench(b *testing.B, r wazero.Runtime) {
	compiled := runCompilation(b, r)
	defer compiled.Close(testCtx)
	// Configure with real sources to avoid performance hit initializing fake ones. These sources are not used
	// in the benchmark.
	config := wazero.NewModuleConfig().
		WithSysNanotime().
		WithSysWalltime().
		WithRandSource(rand.Reader).
		// To measure the pure instantiation time without including calling _start.
		WithStartFunctions().
		WithName("")
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			m, err := r.InstantiateModule(testCtx, compiled, config)
			if err != nil {
				b.Error(err)
			} else {
				m.Close(testCtx)
			}
		}
	})
}

func runAllInvocationBenches(b *testing.B, m api.Module) {
	runBase64Benches(b, m)
	runFibBenches(b, m)
	runStringManipulationBenches(b, m)
	runReverseArrayBenches(b, m)
	runRandomMatMul(b, m)
}

func runBase64Benches(b *testing.B, m api.Module) {
	base64 := m.ExportedFunction("base64")

	for _, numPerExec := range []int{5, 100, 10000} {
		numPerExec := uint64(numPerExec)
		b.ResetTimer()
		b.Run(fmt.Sprintf("base64_%d_per_exec", numPerExec), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := base64.Call(testCtx, numPerExec); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func runFibBenches(b *testing.B, m api.Module) {
	fibonacci := m.ExportedFunction("fibonacci")

	for _, num := range []int{5, 10, 20, 30} {
		num := uint64(num)
		b.ResetTimer()
		b.Run(fmt.Sprintf("fib_for_%d", num), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := fibonacci.Call(testCtx, num); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func runStringManipulationBenches(b *testing.B, m api.Module) {
	stringManipulation := m.ExportedFunction("string_manipulation")

	for _, initialSize := range []int{50, 100, 1000} {
		initialSize := uint64(initialSize)
		b.ResetTimer()
		b.Run(fmt.Sprintf("string_manipulation_size_%d", initialSize), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := stringManipulation.Call(testCtx, initialSize); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func runReverseArrayBenches(b *testing.B, m api.Module) {
	reverseArray := m.ExportedFunction("reverse_array")

	for _, arraySize := range []int{500, 1000, 10000} {
		arraySize := uint64(arraySize)
		b.ResetTimer()
		b.Run(fmt.Sprintf("reverse_array_size_%d", arraySize), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := reverseArray.Call(testCtx, arraySize); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func runRandomMatMul(b *testing.B, m api.Module) {
	randomMatMul := m.ExportedFunction("random_mat_mul")

	for _, matrixSize := range []int{5, 10, 20} {
		matrixSize := uint64(matrixSize)
		b.ResetTimer()
		b.Run(fmt.Sprintf("random_mat_mul_size_%d", matrixSize), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := randomMatMul.Call(testCtx, matrixSize); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func instantiateHostFunctionModuleWithEngine(b *testing.B, config wazero.RuntimeConfig) api.Module {
	r := createRuntime(b, config)

	// Instantiate runs the "_start" function which is what TinyGo compiles "main" to.
	m, err := r.Instantiate(testCtx, caseWasm)
	if err != nil {
		b.Fatal(err)
	}
	return m
}

func createRuntime(b *testing.B, config wazero.RuntimeConfig) wazero.Runtime {
	getRandomString := func(ctx context.Context, m api.Module, retBufPtr uint32, retBufSize uint32) {
		results, err := m.ExportedFunction("allocate_buffer").Call(ctx, 10)
		if err != nil {
			b.Fatal(err)
		}

		offset := uint32(results[0])
		m.Memory().WriteUint32Le(retBufPtr, offset)
		m.Memory().WriteUint32Le(retBufSize, 10)
		b := make([]byte, 10)
		_, _ = rand.Read(b)
		m.Memory().Write(offset, b)
	}

	r := wazero.NewRuntimeWithConfig(testCtx, config)

	_, err := r.NewHostModuleBuilder("env").
		NewFunctionBuilder().WithFunc(getRandomString).Export("get_random_string").
		Instantiate(testCtx)
	if err != nil {
		b.Fatal(err)
	}

	// Note: host_func.go doesn't directly use WASI, but TinyGo needs to be initialized as a WASI Command.
	// Add WASI to satisfy import tests
	wasi_snapshot_preview1.MustInstantiate(testCtx, r)
	return r
}
