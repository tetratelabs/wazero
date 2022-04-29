package experimental_test

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/wasi"
)

type fakeSys struct{}

const (
	epochNanos = uint64(1640995200000000000) // midnight UTC 2022-01-01
	seed       = int64(42)                   // fixed seed value
)

func (d fakeSys) TimeNowUnixNano() uint64 {
	return epochNanos
}

func (d fakeSys) RandSource(p []byte) error {
	s := rand.NewSource(seed)
	rng := rand.New(s)
	_, err := rng.Read(p)
	return err
}

// This is a very basic integration of sys config. The main goal is to show how it is configured.
func Example_sys() {
	// Set context to one that has experimental sys config
	ctx := context.WithValue(context.Background(), experimental.SysKey{}, &fakeSys{})

	r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter())
	wm, err := wasi.InstantiateSnapshotPreview1(ctx, r)
	if err != nil {
		log.Fatal(err)
	}
	defer wm.Close(ctx)

	cfg := wazero.NewModuleConfig().WithStdout(os.Stdout)
	mod, err := r.InstantiateModuleFromCodeWithConfig(ctx, []byte(`(module
  (import "wasi_snapshot_preview1" "random_get"
    (func $wasi.random_get (param $buf i32) (param $buf_len i32) (result (;errno;) i32)))
  (func i32.const 0 i32.const 4 call 0 drop) ;; write 4 bytes of random data
  (memory 1 1)
  (start 1) ;; call the second function
)`), cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer mod.Close(ctx)

	// Try to read 4 bytes of random data.
	if bytes, ok := mod.Memory().Read(ctx, 0, 4); !ok {
		log.Fatalf("Memory.Read(0, 4) out of range of memory size %d", mod.Memory().Size(ctx))
	} else {
		fmt.Println(hex.EncodeToString(bytes))
	}

	// Output:
	// 538c7f96
}
