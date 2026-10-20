package experimental_test

import (
	"context"
	"fmt"
	"log"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
)

// addWasm is (module (func (export "add") (param i32 i32) (result i32) local.get 0 local.get 1 i32.add))
var addWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // magic and version
	0x01, 0x07, 0x01, 0x60, 0x02, 0x7f, 0x7f, 0x01, 0x7f, // type section: (i32, i32) -> i32
	0x03, 0x02, 0x01, 0x00, // function section
	0x07, 0x07, 0x01, 0x03, 'a', 'd', 'd', 0x00, 0x00, // export section: "add"
	0x0a, 0x09, 0x01, 0x07, 0x00, 0x20, 0x00, 0x20, 0x01, 0x6a, 0x0b, // code section
}

// This shows how to make the interpreter lower each function on its first call
// instead of lowering the whole module in CompileModule.
func ExampleWithInterpreterLazyLowering() {
	// Lazy lowering is decided when the module is compiled, so set it on the
	// context passed to CompileModule (or Instantiate).
	ctx := experimental.WithInterpreterLazyLowering(context.Background())

	r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter())
	defer r.Close(ctx)

	// Validates the whole module, but lowers no function bodies yet.
	compiled, err := r.CompileModule(ctx, addWasm)
	if err != nil {
		log.Panicln(err)
	}

	mod, err := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	if err != nil {
		log.Panicln(err)
	}

	// "add" is lowered on this first call, and reused by later calls.
	results, err := mod.ExportedFunction("add").Call(ctx, 1, 2)
	if err != nil {
		log.Panicln(err)
	}
	fmt.Println(results[0])

	// Output:
	// 3
}
