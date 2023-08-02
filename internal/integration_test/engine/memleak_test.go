package adhoc

import (
	"context"
	"log"
	"runtime"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
)

func TestMemoryLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory leak test in short mode.")
	}

	duration := 5 * time.Second
	t.Logf("running memory leak test for %s", duration)

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	for ctx.Err() == nil {
		if err := testMemoryLeakInstantiateRuntimeAndModule(); err != nil {
			log.Panicln(err)
		}
	}

	var stats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&stats)

	if stats.Alloc > (100 * 1024 * 1024) {
		t.Errorf("wazero used more than 100 MiB after running the test for %s (alloc=%d)", duration, stats.Alloc)
	}
}

func testMemoryLeakInstantiateRuntimeAndModule() error {
	ctx := context.Background()

	runtime := wazero.NewRuntime(ctx)
	defer runtime.Close(ctx)

	mod, err := runtime.InstantiateWithConfig(ctx, memoryWasm,
		wazero.NewModuleConfig().WithStartFunctions())
	if err != nil {
		return err
	}
	return mod.Close(ctx)
}
