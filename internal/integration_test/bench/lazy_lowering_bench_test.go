package bench

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// BenchmarkInterpreterLazyLowering compares eager and lazy lowering on WASI
// programs that run a single short command, so only part of their code is
// executed. Each iteration compiles the module in a new runtime and runs it.
// retained-KiB is the heap held after the run while the compiled module is
// still alive.
func BenchmarkInterpreterLazyLowering(b *testing.B) {
	modules := []struct{ name, path string }{
		{"zig-cc", "../../../imports/wasi_snapshot_preview1/testdata/zig-cc/wasi.wasm"},
		{"tinygo", "../../../imports/wasi_snapshot_preview1/testdata/tinygo/wasi.wasm"},
		{"cargo-wasi", "../../../imports/wasi_snapshot_preview1/testdata/cargo-wasi/wasi.wasm"},
	}
	modes := []struct {
		name string
		ctx  context.Context
	}{
		{"eager", testCtx},
		{"lazy", experimental.WithInterpreterLazyLowering(testCtx)},
	}
	for _, m := range modules {
		bin, err := os.ReadFile(m.path)
		if err != nil {
			b.Fatal(err)
		}
		for _, mode := range modes {
			b.Run(m.name+"/"+mode.name, func(b *testing.B) {
				ctx := mode.ctx
				root := b.TempDir()

				// Measure retained memory in an untimed run, as it forces GCs.
				before := heapInUse()
				r, compiled := compileAndRunWASI(b, ctx, bin, root)
				retained := heapInUse() - before
				runtime.KeepAlive(compiled)
				_ = r.Close(ctx)

				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					r, _ := compileAndRunWASI(b, ctx, bin, root)
					_ = r.Close(ctx)
				}
				b.StopTimer()
				b.ReportMetric(float64(retained)/1024, "retained-KiB")
			})
		}
	}
}

// compileAndRunWASI compiles bin in a new interpreter runtime and runs its
// "stat" command with root mounted as "/".
func compileAndRunWASI(b *testing.B, ctx context.Context, bin []byte, root string) (wazero.Runtime, wazero.CompiledModule) {
	r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter())
	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	compiled, err := r.CompileModule(ctx, bin)
	if err != nil {
		b.Fatal(err)
	}
	if _, err = r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithArgs("wasi", "stat").WithFSConfig(wazero.NewFSConfig().WithDirMount(root, "/"))); err != nil {
		b.Fatal(err)
	}
	return r, compiled
}

func heapInUse() int64 {
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return int64(stats.HeapAlloc)
}
