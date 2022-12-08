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
	ctx := context.Background()
	require.False(t, experimental.DWARFBasedStackTraceEnabled(ctx))
	ctx = experimental.WithDWARFBasedStackTrace(ctx)
	require.True(t, experimental.DWARFBasedStackTraceEnabled(ctx))

	type testCase struct {
		name string
		r    wazero.Runtime
	}

	tests := []testCase{{name: "interpreter", r: wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter())}}

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

			for _, lang := range []struct {
				name string
				bin  []byte
				exps []string
			}{
				{
					name: "tinygo",
					bin:  dwarftestdata.TinyGoWasm,
					exps: []string{
						"src/runtime/runtime_tinygowasm.go:73:6",
						"wazero/internal/testing/dwarftestdata/testdata/main.go:19:7",
						"wazero/internal/testing/dwarftestdata/testdata/main.go:14:3",
						"wazero/internal/testing/dwarftestdata/testdata/main.go:9:3",
						"wazero/internal/testing/dwarftestdata/testdata/main.go:4:3",
						"wazero/internal/testing/dwarftestdata/testdata/main.go:4:3",
						"src/runtime/scheduler_none.go:26:10",
						"src/runtime/runtime_wasm_wasi.go:22:5",
					},
				},
				{
					name: "zig",
					bin:  dwarftestdata.ZigWasm,
					exps: []string{
						"lib/std/os.zig:552:9",
						"lib/std/builtin.zig:787:25",
						"main.zig:1:23",
						"lib/std/start.zig:614:37",
					},
				},
			} {
				t.Run(lang.name, func(t *testing.T) {
					compiled, err := r.CompileModule(ctx, lang.bin)
					require.NoError(t, err)

					// Use context.Background to ensure that DWARF is a compile-time option.
					_, err = r.InstantiateModule(context.Background(), compiled, wazero.NewModuleConfig())
					require.Error(t, err)

					errStr := err.Error()
					for _, exp := range lang.exps {
						require.Contains(t, errStr, exp)
					}
				})
			}
		})
	}
}
