package experimental_test

import (
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/dwarftestdata"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestWithDWARFBasedStackTrace(t *testing.T) {
	ctx := experimental.WithDWARFBasedStackTrace(context.Background())

	type testCase struct {
		name string
		r    wazero.Runtime
	}

	tests := []testCase{
		{name: "interpreter", r: wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter())},
	}

	if platform.CompilerSupported() {
		tests = append(tests, testCase{
			name: "compiler", r: wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler()),
		})
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := tc.r
			defer r.Close(ctx) // This closes everything this Runtime created.

			wasi_snapshot_preview1.MustInstantiate(ctx, r)

			_, err := r.InstantiateModuleFromBinary(ctx, dwarftestdata.DWARFWasm)
			require.Error(t, err)
			errStr := err.Error()
			t.Log(err)
			require.Contains(t, errStr, "src/runtime/runtime_tinygowasm.go:73:6")
			require.Contains(t, errStr, "wazero/internal/testing/dwarftestdata/testdata/main.go:19:7")
			require.Contains(t, errStr, "wazero/internal/testing/dwarftestdata/testdata/main.go:14:3")
			require.Contains(t, errStr, "wazero/internal/testing/dwarftestdata/testdata/main.go:9:3")
			require.Contains(t, errStr, "wazero/internal/testing/dwarftestdata/testdata/main.go:4:3")
			require.Contains(t, errStr, "wazero/internal/testing/dwarftestdata/testdata/main.go:4:3")
			require.Contains(t, errStr, "src/runtime/scheduler_none.go:26:10")
			require.Contains(t, errStr, "src/runtime/runtime_wasm_wasi.go:22:5")
		})
	}
}
