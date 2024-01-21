package adhoc

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental/opt"
)

func TestMemoryLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory leak test in short mode.")
	}

	for _, tc := range []struct {
		name     string
		isWazevo bool
	}{
		{"compiler", false},
		{"wazevo", true},
	} {
		tc := tc

		if tc.isWazevo && runtime.GOARCH != "arm64" {
			t.Skip("skipping wazevo memory leak test on non-arm64.")
		}

		t.Run(tc.name, func(t *testing.T) {
			duration := 5 * time.Second
			t.Logf("running memory leak test for %s", duration)

			ctx, cancel := context.WithTimeout(context.Background(), duration)
			defer cancel()

			for ctx.Err() == nil {
				if err := testMemoryLeakInstantiateRuntimeAndModule(tc.isWazevo); err != nil {
					log.Panicln(err)
				}
			}

			var stats runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&stats)

			if stats.Alloc > (100 * 1024 * 1024) {
				t.Errorf("wazero used more than 100 MiB after running the test for %s (alloc=%d)", duration, stats.Alloc)
			}
			fmt.Println(stats.Alloc)
		})
	}
}

func testMemoryLeakInstantiateRuntimeAndModule(isWazevo bool) error {
	ctx := context.Background()

	var r wazero.Runtime
	if isWazevo {
		c := opt.NewRuntimeConfigOptimizingCompiler()
		r = wazero.NewRuntimeWithConfig(ctx, c)
	} else {
		r = wazero.NewRuntime(ctx)
	}
	defer r.Close(ctx)

	hostBuilder := r.NewHostModuleBuilder("test")

	for i := 0; i < 100; i++ {
		hostBuilder.NewFunctionBuilder().WithFunc(func() uint32 { return uint32(i) }).Export(strconv.Itoa(i))
	}

	hostMod, err := hostBuilder.Instantiate(ctx)
	if err != nil {
		return err
	}
	if err = hostMod.Close(ctx); err != nil {
		return err
	}

	mod, err := r.InstantiateWithConfig(ctx, memoryWasm,
		wazero.NewModuleConfig().WithStartFunctions())
	if err != nil {
		return err
	}
	return mod.Close(ctx)
}
