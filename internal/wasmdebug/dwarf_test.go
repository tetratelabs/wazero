package wasmdebug_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/dwarftestdata"
	"github.com/tetratelabs/wazero/internal/testing/hammer"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
)

func TestDWARFLines_Line_Zig(t *testing.T) {
	mod, err := binary.DecodeModule(dwarftestdata.ZigWasm, api.CoreFeaturesV2, wasm.MemoryLimitPages, false, true, false)
	require.NoError(t, err)
	require.NotNil(t, mod.DWARFLines)

	// codeSecStart is the beginning of the code section in the Wasm binary.
	// If dwarftestdata.ZigWasm has been changed, we need to inspect by `wasm-tools objdump`.
	const codeSecStart = 0x48

	// These cases are crafted by matching the stack trace result from wasmtime. To verify, run:
	//
	// 	WASMTIME_BACKTRACE_DETAILS=1 wasmtime run internal/testing/dwarftestdata/testdata/zig/main.wasm
	//
	// And this should produce the output as:
	//
	// Caused by:
	// 0: failed to invoke command default
	// 1: error while executing at wasm backtrace:
	//        0:   0xab - builtin.default_panic
	//                        at /opt/homebrew/Cellar/zig/0.13.0/lib/zig/std/builtin.zig:792:17
	//        1:   0x6d - main.inlined_b
	//                        at /Users/anuraag/git/wazero/internal/testing/dwarftestdata/testdata/zig/main.zig:10:5              - main.inlined_a
	//                        at /Users/anuraag/git/wazero/internal/testing/dwarftestdata/testdata/zig/main.zig:6:5              - main.main
	//                        at /Users/anuraag/git/wazero/internal/testing/dwarftestdata/testdata/zig/main.zig:2:5
	//        2:  0x119 - start.callMain
	//                        at /opt/homebrew/Cellar/zig/0.13.0/lib/zig/std/start.zig:524:37              - start.wasm_freestanding_start
	//                        at /opt/homebrew/Cellar/zig/0.13.0/lib/zig/std/start.zig:199:5
	// 2: wasm trap: wasm `unreachable` instruction executed
	for _, tc := range []struct {
		offset uint64
		exp    []string
	}{
		{offset: 0xab - codeSecStart, exp: []string{
			"lib/zig/std/builtin.zig:792:17",
		}},
		{offset: 0x6d - codeSecStart, exp: []string{
			"zig/main.zig:10:5 (inlined)",
			"zig/main.zig:6:5 (inlined)",
			"zig/main.zig:2:5",
		}},
		{offset: 0x119 - codeSecStart, exp: []string{
			"lib/zig/std/start.zig:524:37 (inlined)",
			"lib/zig/std/start.zig:199:5",
		}},
	} {
		tc := tc
		t.Run(fmt.Sprintf("%#x/%s", tc.offset, tc.exp), func(t *testing.T) {
			// Ensures that DWARFLines.Line is goroutine-safe.
			hammer.NewHammer(t, 100, 5).Run(func(p, n int) {
				actual := mod.DWARFLines.Line(tc.offset)
				require.Equal(t, len(tc.exp), len(actual))
				for i := range tc.exp {
					require.Contains(t, actual[i], tc.exp[i])
				}
			}, nil)
		})
	}
}

func TestDWARFLines_Line_Rust(t *testing.T) {
	if len(dwarftestdata.RustWasm) == 0 {
		t.Skip()
	}
	mod, err := binary.DecodeModule(dwarftestdata.RustWasm, api.CoreFeaturesV2, wasm.MemoryLimitPages, false, true, false)
	require.NoError(t, err)
	require.NotNil(t, mod.DWARFLines)

	// codeSecStart is the beginning of the code section in the Wasm binary.
	// If dwarftestdata.RustWasm has been changed, we need to inspect by `wasm-tools objdump`.
	const codeSecStart = 0x20d

	// These cases are crafted by matching the stack trace result from wasmtime. To verify, run:
	//
	// 	WASMTIME_BACKTRACE_DETAILS=1 wasmtime run internal/testing/dwarftestdata/testdata/rust/main.wasm
	//
	// And this should produce the output as:
	// Caused by:
	// 0: failed to invoke command default
	// 1: error while executing at wasm backtrace:
	//        0: 0x3483 - panic_abort::__rust_start_panic::abort::hd05f7e510a9bfacb
	//                        at /rustc/9fc6b43126469e3858e2fe86cafb4f0fd5068869/library/panic_abort/src/lib.rs:100:17              - __rust_start_panic
	//                        at /rustc/9fc6b43126469e3858e2fe86cafb4f0fd5068869/library/panic_abort/src/lib.rs:43:5
	//        1: 0x33d5 - rust_panic
	//                        at /rustc/9fc6b43126469e3858e2fe86cafb4f0fd5068869/library/std/src/panicking.rs:862:25
	//        2: 0x33a8 - std::panicking::rust_panic_with_hook::hf4c55e90d4731159
	//                        at /rustc/9fc6b43126469e3858e2fe86cafb4f0fd5068869/library/std/src/panicking.rs:826:5
	//        3: 0x27ef - std::panicking::begin_panic_handler::{{closure}}::h9e9ba254d816924b
	//                        at /rustc/9fc6b43126469e3858e2fe86cafb4f0fd5068869/library/std/src/panicking.rs:667:13
	//        4: 0x2729 - std::sys::backtrace::__rust_end_short_backtrace::h5fb21e191bc452e3
	//                        at /rustc/9fc6b43126469e3858e2fe86cafb4f0fd5068869/library/std/src/sys/backtrace.rs:170:18
	//        5: 0x2d3c - rust_begin_unwind
	//                        at /rustc/9fc6b43126469e3858e2fe86cafb4f0fd5068869/library/std/src/panicking.rs:665:5
	//        6: 0x7b97 - core::panicking::panic_fmt::hfe24bec0337a4754
	//                        at /rustc/9fc6b43126469e3858e2fe86cafb4f0fd5068869/library/core/src/panicking.rs:76:14
	//        7:  0x580 - main::b::h7fe25a8329542864
	//                        at /Users/anuraag/git/wazero/internal/testing/dwarftestdata/testdata/rust/main.rs:12:5              - main::a::hf5dc043bad87cf46
	//                        at /Users/anuraag/git/wazero/internal/testing/dwarftestdata/testdata/rust/main.rs:7:5              - main::main::hef810e4bf58d9cdf
	//                        at /Users/anuraag/git/wazero/internal/testing/dwarftestdata/testdata/rust/main.rs:2:5
	//        8:  0x331 - core::ops::function::FnOnce::call_once::hb3419529f8e10fb1
	//                        at /Users/anuraag/.rustup/toolchains/stable-aarch64-apple-darwin/lib/rustlib/src/rust/library/core/src/ops/function.rs:250:5
	//        9:  0x496 - std::sys::backtrace::__rust_begin_short_backtrace::h6b7139fa671fb72e
	//                        at /Users/anuraag/.rustup/toolchains/stable-aarch64-apple-darwin/lib/rustlib/src/rust/library/std/src/sys/backtrace.rs:154:18
	//       10:  0x419 - std::rt::lang_start::{{closure}}::hefb60d097516fc9f
	//                        at /Users/anuraag/.rustup/toolchains/stable-aarch64-apple-darwin/lib/rustlib/src/rust/library/std/src/rt.rs:195:18
	//       11: 0x198d - core::ops::function::impls::<impl core::ops::function::FnOnce<A> for &F>::call_once::h13726548ebbd9f5e
	//                        at /rustc/9fc6b43126469e3858e2fe86cafb4f0fd5068869/library/core/src/ops/function.rs:284:13              - std::panicking::try::do_call::h486344993a14f2c0
	//                        at /rustc/9fc6b43126469e3858e2fe86cafb4f0fd5068869/library/std/src/panicking.rs:557:40              - std::panicking::try::hedadb8cd413b03f6
	//                        at /rustc/9fc6b43126469e3858e2fe86cafb4f0fd5068869/library/std/src/panicking.rs:520:19              - std::panic::catch_unwind::hc1f7f9244b2fb00b
	//                        at /rustc/9fc6b43126469e3858e2fe86cafb4f0fd5068869/library/std/src/panic.rs:358:14              - std::rt::lang_start_internal::{{closure}}::hfb0e2b398e86f6de
	//                        at /rustc/9fc6b43126469e3858e2fe86cafb4f0fd5068869/library/std/src/rt.rs:174:48              - std::panicking::try::do_call::hebcae3b56ebbc340
	//                        at /rustc/9fc6b43126469e3858e2fe86cafb4f0fd5068869/library/std/src/panicking.rs:557:40              - std::panicking::try::h8850f6913d04c130
	//                        at /rustc/9fc6b43126469e3858e2fe86cafb4f0fd5068869/library/std/src/panicking.rs:520:19              - std::panic::catch_unwind::hf3726b3a12c06aad
	//                        at /rustc/9fc6b43126469e3858e2fe86cafb4f0fd5068869/library/std/src/panic.rs:358:14              - std::rt::lang_start_internal::h1fceb22bbe5297a1
	//                        at /rustc/9fc6b43126469e3858e2fe86cafb4f0fd5068869/library/std/src/rt.rs:174:20
	//       12:  0x3b5 - std::rt::lang_start::he470b12ea6d4e370
	//                        at /Users/anuraag/.rustup/toolchains/stable-aarch64-apple-darwin/lib/rustlib/src/rust/library/std/src/rt.rs:194:17
	//       13:  0x5a4 - main-144f120e836a09da.wasm!__main_void
	//       14:  0x244 - _start
	//                        at wasisdk://v23.0/build/sysroot/wasi-libc-wasm32-wasip1/libc-bottom-half/crt/crt1-command.c:43:13
	// 2: wasm trap: wasm `unreachable` instruction executed
	for _, tc := range []struct {
		offset uint64
		exp    []string
	}{
		{offset: 0x3483 - codeSecStart, exp: []string{
			"/library/panic_abort/src/lib.rs:100:17",
			"/library/panic_abort/src/lib.rs:43:5",
		}},
		{offset: 0x580 - codeSecStart, exp: []string{
			"/wazero/internal/testing/dwarftestdata/testdata/rust/main.rs:12:5",
			"/wazero/internal/testing/dwarftestdata/testdata/rust/main.rs:7:5",
			"/wazero/internal/testing/dwarftestdata/testdata/rust/main.rs:2:5",
		}},
		{offset: 0x496 - codeSecStart, exp: []string{
			"/library/std/src/sys/backtrace.rs:154:18",
		}},
	} {
		tc := tc
		t.Run(fmt.Sprintf("%#x/%s", tc.offset, tc.exp), func(t *testing.T) {
			actual := mod.DWARFLines.Line(tc.offset)

			require.Equal(t, len(tc.exp), len(actual))
			for i := range tc.exp {
				require.Contains(t, actual[i], tc.exp[i])
			}
		})
	}
}

func TestDWARFLines_Line_TinyGo(t *testing.T) {
	mod, err := binary.DecodeModule(dwarftestdata.TinyGoWasm, api.CoreFeaturesV2, wasm.MemoryLimitPages, false, true, false)
	require.NoError(t, err)
	require.NotNil(t, mod.DWARFLines)

	// codeSecStart is the beginning of the code section in the Wasm binary.
	// If dwarftestdata.TinyGoWasm has been changed, we need to inspect by `wasm-tools objdump`.
	const codeSecStart = 0x16f

	// These cases are crafted by matching the stack trace result from wasmtime. To verify, run:
	//
	// 	WASMTIME_BACKTRACE_DETAILS=1 wasmtime run internal/testing/dwarftestdata/testdata/tinygo/main.wasm
	//
	// And this should produce the output as:
	//
	// Caused by:
	//    0: failed to invoke command default
	//    1: error while executing at wasm backtrace:
	//           0: 0x1a62 - runtime.abort
	//                           at /Users/mathetake/Downloads/tinygo/src/runtime/runtime_tinygowasm.go:70:6              - runtime._panic
	//                           at /Users/mathetake/Downloads/tinygo/src/runtime/panic.go:52:7
	//           1: 0x3168 - main.c
	//                           at /Users/mathetake/Downloads/tinygo/tmo/main.go:16:7
	//           2: 0x3106 - main.b
	//                           at /Users/mathetake/Downloads/tinygo/tmo/main.go:12:3
	//           3: 0x30a8 - main.a
	//                           at /Users/mathetake/Downloads/tinygo/tmo/main.go:8:3
	//           4: 0x22b8 - main.main
	//                           at /Users/mathetake/Downloads/tinygo/tmo/main.go:4:3
	//           5: 0x213a - runtime.run$1
	//                           at /Users/mathetake/Downloads/tinygo/src/runtime/scheduler_any.go:25:11
	//           6:  0x85f - <goroutine wrapper>
	//                           at /Users/mathetake/Downloads/tinygo/src/runtime/scheduler_any.go:23:2
	//           7:  0x192 - tinygo_launch
	//                           at /Users/mathetake/Downloads/tinygo/src/internal/task/task_asyncify_wasm.S:59
	//           8: 0x2033 - (*internal/task.Task).Resume
	//                           at /Users/mathetake/Downloads/tinygo/src/internal/task/task_asyncify.go:109:17              - runtime.scheduler
	//                           at /Users/mathetake/Downloads/tinygo/src/runtime/scheduler.go:236:11
	//           9: 0x1f01 - runtime.run
	//                           at /Users/mathetake/Downloads/tinygo/src/runtime/scheduler_any.go:28:11
	//          10: 0x1e81 - _start
	//                           at /Users/mathetake/Downloads/tinygo/src/runtime/runtime_wasm_wasi.go:21:5
	for _, tc := range []struct {
		offset uint64
		exp    []string
	}{
		{offset: 0x1e81 - codeSecStart, exp: []string{"runtime/runtime_wasm_wasi.go:21:5"}},
		{offset: 0x1f01 - codeSecStart, exp: []string{"runtime/scheduler_any.go:28:11"}},
		{offset: 0x2033 - codeSecStart, exp: []string{
			"internal/task/task_asyncify.go:109:17",
			"runtime/scheduler.go:236:11",
		}},
		{offset: 0x192 - codeSecStart, exp: []string{"internal/task/task_asyncify_wasm.S:59"}},
		{offset: 0x85f - codeSecStart, exp: []string{"runtime/scheduler_any.go:23:2"}},
		{offset: 0x213a - codeSecStart, exp: []string{"runtime/scheduler_any.go:25:11"}},
		{offset: 0x22b8 - codeSecStart, exp: []string{"main.go:4:3"}},
		{offset: 0x30a8 - codeSecStart, exp: []string{"main.go:8:3"}},
		{offset: 0x3106 - codeSecStart, exp: []string{"main.go:12:3"}},
		{offset: 0x3168 - codeSecStart, exp: []string{"main.go:16:7"}},
		// Note(important): this case is different from the output of Wasmtime, which produces the incorrect inline info (panic.go:52:7).
		// Actually, "runtime_tinygowasm.go:70:6" invokes trap() which is translated as "unreachable" instruction by LLVM, so there won't be
		// any inlined function invocation here.
		{offset: 0x1a62 - codeSecStart, exp: []string{"runtime/runtime_tinygowasm.go:70:6"}},
	} {
		tc := tc
		t.Run(fmt.Sprintf("%#x/%s", tc.offset, tc.exp), func(t *testing.T) {
			actual := mod.DWARFLines.Line(tc.offset)
			require.Equal(t, len(tc.exp), len(actual), "\nexp: %s\ngot: %s", strings.Join(tc.exp, "\n"), strings.Join(actual, "\n"))
			for i := range tc.exp {
				require.Contains(t, actual[i], tc.exp[i])
			}
		})
	}
}
