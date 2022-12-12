package experimental_test

import (
	"bufio"
	"context"
	_ "embed"
	"strings"
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
				exp  string
			}{
				{
					name: "tinygo",
					bin:  dwarftestdata.TinyGoWasm,
					exp: `module[] function[_start] failed: wasm error: unreachable
wasm stack trace:
	.runtime._panic(i32)
		0x16e2: /runtime_tinygowasm.go:73:6 (inlined)
		        /panic.go:52:7
	.c()
		0x1919: /main.go:19:7
	.b()
		0x1901: /main.go:14:3
	.a()
		0x18f7: /main.go:9:3
	.main.main()
		0x18ed: /main.go:4:3
	.runtime.run()
		0x18cc: /scheduler_none.go:26:10
	._start()
		0x18b6: /runtime_wasm_wasi.go:22:5`,
				},
				{
					name: "zig",
					bin:  dwarftestdata.ZigWasm,
					exp: `module[] function[_start] failed: wasm error: unreachable
wasm stack trace:
	.os.abort()
		0x1b3: /os.zig:552:9
	.builtin.default_panic(i32,i32,i32,i32)
		0x86: /builtin.zig:787:25
	.main.main() i32
		0x2d: main.zig:10:5 (inlined)
		      main.zig:6:5 (inlined)
		      main.zig:2:5
	._start()
		0x1c6: /start.zig:614:37 (inlined)
		       /start.zig:240:42`,
				},
			} {
				t.Run(lang.name, func(t *testing.T) {
					compiled, err := r.CompileModule(ctx, lang.bin)
					require.NoError(t, err)

					// Use context.Background to ensure that DWARF is a compile-time option.
					_, err = r.InstantiateModule(context.Background(), compiled, wazero.NewModuleConfig())
					require.Error(t, err)

					errStr := err.Error()

					// Since stack traces change where the binary is compiled, we sanitize each line
					// so that it doesn't contain any file system dependent info.
					scanner := bufio.NewScanner(strings.NewReader(errStr))
					scanner.Split(bufio.ScanLines)
					var sanitizedLines []string
					for scanner.Scan() {
						line := scanner.Text()
						start, last := strings.Index(line, "/"), strings.LastIndex(line, "/")
						if start >= 0 {
							l := len(line) - last
							buf := []byte(line)
							copy(buf[start:], buf[last:])
							line = string(buf[:start+l])
						}
						sanitizedLines = append(sanitizedLines, line)
					}

					sanitizedTraces := strings.Join(sanitizedLines, "\n")
					require.Equal(t, sanitizedTraces, lang.exp)
				})
			}
		})
	}
}
