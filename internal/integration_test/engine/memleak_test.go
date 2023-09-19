package adhoc

import (
	"context"
	"log"
	"runtime"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/engine/wazevo"
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
		})
	}
}

func testMemoryLeakInstantiateRuntimeAndModule(isWazevo bool) error {
	ctx := context.Background()

	var r wazero.Runtime
	if isWazevo {
		c := wazero.NewRuntimeConfigInterpreter()
		wazevo.ConfigureWazevo(c)
		r = wazero.NewRuntimeWithConfig(ctx, c)
	} else {
		r = wazero.NewRuntime(ctx)
	}
	defer r.Close(ctx)

	mod, err := r.InstantiateWithConfig(ctx, memoryWasm,
		wazero.NewModuleConfig().WithStartFunctions())
	if err != nil {
		return err
	}
	return mod.Close(ctx)
}
