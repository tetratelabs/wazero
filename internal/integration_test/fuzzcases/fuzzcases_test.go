package fuzzcases

import (
	"context"
	_ "embed"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"testing"

	"github.com/tetratelabs/wazero"
)

var ctx = context.Background()

var (
	//go:embed testdata/695.wasm
	case695 []byte
)

func newRuntimeCompiler() wazero.Runtime {
	return wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigCompiler().WithWasmCore2())
}

func newRuntimeInterpreter() wazero.Runtime {
	return wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter().WithWasmCore2())
}

// Test695 requires two functions to exit with "out of bounds memory access" consistently across the implementations.
func Test695(t *testing.T) {
	for _, tc := range []struct {
		name string
		r    wazero.Runtime
	}{
		{name: "compiler", r: newRuntimeCompiler()},
		{name: "interpreter", r: newRuntimeInterpreter()},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			defer tc.r.Close(ctx)
			module, err := tc.r.InstantiateModuleFromBinary(ctx, case695)
			require.NoError(t, err)

			_, err = module.ExportedFunction("i8x16s").Call(ctx)
			require.Contains(t, err.Error(), "out of bounds memory access")

			_, err = module.ExportedFunction("i16x8s").Call(ctx)
			require.Contains(t, err.Error(), "out of bounds memory access")
		})
	}
}
