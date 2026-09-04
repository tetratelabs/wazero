package adhoc

// Exception handling + GC integration tests.
//
// Ported from https://github.com/bytecodealliance/endive/blob/d98c0b3d/wasm-corpus/src/main/resources/wat/exception_gc_ref.wat
// Test expectations from https://github.com/bytecodealliance/endive/blob/d98c0b3d/machine-tests/src/test/java/run/endive/testing/ExceptionGcRefTest.java

import (
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

//go:embed testdata/eh_gc_ref.wasm
var ehGcRefWasm []byte

func TestEHGcRefInterpreter(t *testing.T) {
	cfg := wazero.NewRuntimeConfigInterpreter().
		WithCoreFeatures(api.CoreFeaturesV2 |
			experimental.CoreFeaturesExceptionHandling |
			experimental.CoreFeaturesGC)
	runEHGcRefTests(t, cfg)
}

func runEHGcRefTests(t *testing.T, cfg wazero.RuntimeConfig) {
	for _, tc := range []struct {
		name string
		want int32
	}{
		{"basic-catch-gc", 42},
		{"sequential-catches-gc", 30},
		{"catch-from-call-gc", 30},
		{"deep-catch-gc", 30},
		{"catch-in-loop-gc", 4},
		{"deep-catch-in-loop-gc", 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			r := wazero.NewRuntimeWithConfig(ctx, cfg)
			defer r.Close(ctx)

			_, err := r.NewHostModuleBuilder("host").
				NewFunctionBuilder().
				WithFunc(func(v int32) int32 { return v }).
				Export("on_catch").
				Instantiate(ctx)
			require.NoError(t, err)

			mod, err := r.InstantiateWithConfig(ctx, ehGcRefWasm,
				wazero.NewModuleConfig().WithStartFunctions())
			require.NoError(t, err)

			res, err := mod.ExportedFunction(tc.name).Call(ctx)
			require.NoError(t, err)
			require.Equal(t, tc.want, api.DecodeI32(res[0]))
		})
	}
}
