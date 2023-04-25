package wazero_test

import (
	"bufio"
	"context"
	_ "embed"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/dwarftestdata"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestWithDebugInfo(t *testing.T) {
	ctx := context.Background()

	type testCase struct {
		name string
		r    wazero.Runtime
	}

	tests := []testCase{{
		name: "interpreter",
		r:    wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter()),
	}}

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
					name: "zig",
					bin:  dwarftestdata.ZigWasm,
					exp: `module[] function[_start] failed: wasm error: unreachable
wasm stack trace:
	.builtin.default_panic(i32,i32,i32,i32)
		0x37: /builtin.zig:861:17
	.main.main() i32
		0x60: /main.zig:10:5 (inlined)
		      /main.zig:6:5 (inlined)
		      /main.zig:2:5
	._start()
		0x6a: /start.zig:617:37 (inlined)
		      /start.zig:232:5`,
				},
				{
					name: "rust",
					bin:  dwarftestdata.RustWasm,
					exp: `module[] function[_start] failed: wasm error: unreachable
wasm stack trace:
	.__rust_start_panic(i32) i32
		0x60bf: /lib.rs:83:17 (inlined)
		        /lib.rs:37:5
	.rust_panic(i32,i32)
		0x5dad: /panicking.rs:740:9
	rust_panic_with_hook(i32,i32,i32,i32,i32)
		0x5d74: /panicking.rs:710:5
	{closure#0}(i32)
		0x4e2e: /panicking.rs:577:13
	__rust_end_short_backtrace<std::panicking::begin_panic_handler::{closure_env#0}, !>(i32)
		0x4d90: /backtrace.rs:137:18
	begin_panic_handler(i32)
		0x53e2: /panicking.rs:575:5
	panic_fmt(i32,i32)
		0xaab8: /panicking.rs:64:14
	b<i32>(i32)
		0x305: /main.rs:12:5
	main()
		0x5d6: /main.rs:7:5 (inlined)
		       /main.rs:2:5
	call_once<fn(), ()>(i32)
		0x142: /function.rs:250:5
	__rust_begin_short_backtrace<fn(), ()>(i32)
		0x5b1: /backtrace.rs:121:18
	{closure#0}<()>(i32) i32
		0x3d4: /rt.rs:166:18
	lang_start_internal(i32,i32,i32,i32,i32) i32
		0x25d6: /function.rs:287:13 (inlined)
		        /panicking.rs:483:40 (inlined)
		        /panicking.rs:447:19 (inlined)
		        /panic.rs:140:14 (inlined)
		        /rt.rs:148:48 (inlined)
		        /panicking.rs:483:40 (inlined)
		        /panicking.rs:447:19 (inlined)
		        /panic.rs:140:14 (inlined)
		        /rt.rs:148:20
	lang_start<()>(i32,i32,i32,i32) i32
		0x371: /rt.rs:165:17
	.__main_void() i32
	._start()`,
				},
			} {
				t.Run(lang.name, func(t *testing.T) {
					if len(lang.bin) == 0 {
						t.Skip()
					}

					_, err := r.Instantiate(ctx, lang.bin)
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
					require.Equal(t, lang.exp, sanitizedTraces)
				})
			}
		})
	}
}
