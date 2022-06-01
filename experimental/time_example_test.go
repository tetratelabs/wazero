package experimental_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/wasi_snapshot_preview1"
)

const epochNanos = uint64(1640995200000000000) // midnight UTC 2022-01-01

// This is a basic example of overriding the clock via WithTimeNowUnixNano. The main goal is to show how it is configured.
func Example_withTimeNowUnixNano() {
	ctx := context.Background()

	r := wazero.NewRuntime()
	defer r.Close(ctx) // This closes everything this Runtime created.

	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		log.Panicln(err)
	}

	// Instantiate a module that only re-exports a WASI function that uses the clock.
	mod, err := r.InstantiateModuleFromCode(ctx, []byte(`(module
  (import "wasi_snapshot_preview1" "clock_time_get"
    (func $wasi.clock_time_get (param $id i32) (param $precision i64) (param $result.timestamp i32) (result (;errno;) i32)))

  (memory 1 1) ;; memory is required for WASI

  (export "clock_time_get" (func 0))
)`))
	if err != nil {
		log.Panicln(err)
	}

	// Call clock_time_get in context of an experimental clock function
	ctx = experimental.WithTimeNowUnixNano(ctx, func() uint64 { return epochNanos })
	results, err := mod.ExportedFunction("clock_time_get").Call(ctx, 0, 0, 0)
	if err != nil {
		log.Panicln(err)
	}
	if results[0] != 0 {
		log.Panicf("received errno %d\n", results[0])
	}

	// Try to read the time WASI wrote to memory at offset zero.
	if nanos, ok := mod.Memory().ReadUint64Le(ctx, 0); !ok {
		log.Panicf("Memory.ReadUint64Le(0) out of range of memory size %d", mod.Memory().Size(ctx))
	} else {
		fmt.Println(time.UnixMicro(int64(nanos / 1000)).UTC())
	}

	// Output:
	// 2022-01-01 00:00:00 +0000 UTC
}
