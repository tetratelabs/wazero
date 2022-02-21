package bench

import (
	"context"
	_ "embed"
	"fmt"
	"math/rand"
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
)

// caseWasm was compiled from TinyGo testdata/case.go
//go:embed testdata/case.wasm
var caseWasm []byte

func BenchmarkEngines(b *testing.B) {
	b.Run("interpreter", func(b *testing.B) {
		m := instantiateHostFunctionModuleWithEngine(b, wazero.NewEngineInterpreter())
		runAllBenches(b, m)
	})
	if runtime.GOARCH == "amd64" {
		b.Run("jit", func(b *testing.B) {
			m := instantiateHostFunctionModuleWithEngine(b, wazero.NewEngineJIT())
			runAllBenches(b, m)
		})
	}
}

func runAllBenches(b *testing.B, m wasm.ModuleExports) {
	runBase64Benches(b, m)
	runFibBenches(b, m)
	runStringsManipulationBenches(b, m)
	runReverseArrayBenches(b, m)
	runRandomMatMul(b, m)
}

func runBase64Benches(b *testing.B, m wasm.ModuleExports) {
	fn, ok := m.Function("base64")
	if !ok {
		b.Fatal("function base64 not exported")
	}
	ctx := context.Background()
	for _, numPerExec := range []int{5, 100, 10000} {
		numPerExec := numPerExec
		b.ResetTimer()
		b.Run(fmt.Sprintf("base64_%d_per_exec", numPerExec), func(b *testing.B) {
			if _, err := fn(ctx, uint64(numPerExec)); err != nil {
				b.Fatal(err)
			}
		})
	}
}

func runFibBenches(b *testing.B, m wasm.ModuleExports) {
	fn, ok := m.Function("fibonacci")
	if !ok {
		b.Fatal("function base64 not exported")
	}
	ctx := context.Background()
	for _, num := range []int{5, 10, 20, 30} {
		num := num
		b.ResetTimer()
		b.Run(fmt.Sprintf("fib_for_%d", num), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := fn(ctx, uint64(num)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func runStringsManipulationBenches(b *testing.B, m wasm.ModuleExports) {
	fn, ok := m.Function("string_manipulation")
	if !ok {
		b.Fatal("function string_manipulation not exported")
	}
	ctx := context.Background()
	for _, initialSize := range []int{50, 100, 1000} {
		initialSize := initialSize
		b.ResetTimer()
		b.Run(fmt.Sprintf("string_manipulation_size_%d", initialSize), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := fn(ctx, uint64(initialSize)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func runReverseArrayBenches(b *testing.B, m wasm.ModuleExports) {
	fn, ok := m.Function("reverse_array")
	if !ok {
		b.Fatal("function reverse_array not exported")
	}
	ctx := context.Background()
	for _, arraySize := range []int{500, 1000, 10000} {
		arraySize := arraySize
		b.ResetTimer()
		b.Run(fmt.Sprintf("reverse_array_size_%d", arraySize), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := fn(ctx, uint64(arraySize)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func runRandomMatMul(b *testing.B, m wasm.ModuleExports) {
	fn, ok := m.Function("random_mat_mul")
	if !ok {
		b.Fatal("function random_mat_mul not exported")
	}
	ctx := context.Background()
	for _, matrixSize := range []int{5, 10, 20} {
		matrixSize := matrixSize
		b.ResetTimer()
		b.Run(fmt.Sprintf("random_mat_mul_size_%d", matrixSize), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := fn(ctx, uint64(matrixSize)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func instantiateHostFunctionModuleWithEngine(b *testing.B, engine *wazero.Engine) wasm.ModuleExports {
	getRandomString := func(ctx wasm.ModuleContext, retBufPtr uint32, retBufSize uint32) {
		allocateBuffer, ok := ctx.Function("allocate_buffer")
		if !ok {
			b.Fatal("couldn't find function allocate_buffer")
		}

		results, err := allocateBuffer(ctx.Context(), 10)
		if err != nil {
			b.Fatal(err)
		}

		offset := uint32(results[0])
		ctx.Memory().WriteUint32Le(retBufPtr, offset)
		ctx.Memory().WriteUint32Le(retBufSize, 10)
		b := make([]byte, 10)
		_, _ = rand.Read(b)
		ctx.Memory().Write(offset, b)
	}

	store := wazero.NewStoreWithConfig(&wazero.StoreConfig{Engine: engine})

	_, err := wazero.ExportHostFunctions(store, "env", map[string]interface{}{"get_random_string": getRandomString})
	if err != nil {
		b.Fatal(err)
	}

	// Note: host_func.go doesn't directly use WASI, but TinyGo needs to be initialized as a WASI Command.
	_, err = wazero.ExportHostFunctions(store, wasi.ModuleSnapshotPreview1, wazero.WASISnapshotPreview1())
	if err != nil {
		b.Fatal(err)
	}

	mod, err := wazero.DecodeModuleBinary(caseWasm)
	if err != nil {
		b.Fatal(err)
	}

	m, err := wazero.StartWASICommand(store, mod)
	if err != nil {
		b.Fatal(err)
	}
	return m
}
