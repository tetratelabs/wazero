package experimental_test

import (
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

//go:embed testdata/cpp_exceptions.wasm
var cppExceptionsWasm []byte

func TestCppExceptions(t *testing.T) {
	ctx := context.Background()

	cfg := wazero.NewRuntimeConfig().
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling)

	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	mod, err := r.InstantiateWithConfig(ctx, cppExceptionsWasm,
		wazero.NewModuleConfig().WithStartFunctions("_initialize"))
	require.NoError(t, err)

	tests := []struct {
		name     string
		expected int32
	}{
		{"test_no_throw", 42},
		{"test_catch_specific", -1},
		{"test_catch_base", 1},
		{"test_rethrow", -42},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := mod.ExportedFunction(tc.name).Call(ctx)
			require.NoError(t, err)
			require.Equal(t, tc.expected, api.DecodeI32(res[0]))
		})
	}
}
