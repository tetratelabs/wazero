package experimental_test

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/wasi"
)

type logger struct{ funcName string }

func (l *logger) Before(ctx context.Context, _ []uint64) context.Context {
	fmt.Println(">>", l.funcName)
	return ctx
}

func (l *logger) After(context.Context, []uint64) {
	fmt.Println("<<", l.funcName)
}

type loggerFactory struct{}

func (f *loggerFactory) NewListener(fnd experimental.FunctionDefinition) experimental.FunctionListener {
	return &logger{funcName: fnd.ModuleName() + "." + funcName(fnd)}
}

// funcName returns the name in priority order: first export name, module-defined name, numeric index.
func funcName(fnd experimental.FunctionDefinition) string {
	if len(fnd.ExportNames()) > 0 {
		return fnd.ExportNames()[0]
	}
	if fnd.Name() != "" {
		return fnd.Name()
	}
	return fmt.Sprintf("[%d]", fnd.Index())
}

// This is a very basic integration of listener. The main goal is to show how it is configured.
func Example_listener() {
	// Set context to one that has an experimental listener
	ctx := context.WithValue(context.Background(), experimental.FunctionListenerFactoryKey{}, &loggerFactory{})

	r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter())
	wm, err := wasi.InstantiateSnapshotPreview1(ctx, r)
	if err != nil {
		log.Fatal(err)
	}
	defer wm.Close(ctx)

	cfg := wazero.NewModuleConfig().WithStdout(os.Stdout)
	mod, err := r.InstantiateModuleFromCodeWithConfig(ctx, []byte(`(module $listener
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

	// Output:
	// >> listener.[1]
	// >> wasi_snapshot_preview1.random_get
	// << wasi_snapshot_preview1.random_get
	// << listener.[1]
}
