package wasmdebug_test

import (
	"fmt"
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
	const codeSecStart = 0x46

	// These cases are crafted by matching the stack trace result from wasmtime. To verify, run:
	//
	// 	WASMTIME_BACKTRACE_DETAILS=1 wasmtime run internal/testing/dwarftestdata/testdata/zig/main.wasm
	//
	// And this should produce the output as:
	//
	// Caused by:
	//    0: failed to invoke command default
	//    1: error while executing at wasm backtrace:
	//           0:   0x7d - builtin.default_panic
	//                           at /Users/adrian/Downloads/zig-macos-x86_64-0.11.0-dev.1499+23b7d2889/lib/std/builtin.zig:861:17
	//           1:   0xa6 - main.inlined_b
	//                           at /Users/adrian/oss/wazero/internal/testing/dwarftestdata/testdata/zig/main.zig:10:5              - main.inlined_a
	//                           at /Users/adrian/oss/wazero/internal/testing/dwarftestdata/testdata/zig/main.zig:6:5              - main.main
	//                           at /Users/adrian/oss/wazero/internal/testing/dwarftestdata/testdata/zig/main.zig:2:5
	//           2:   0xb0 - start.callMain
	//                           at /Users/adrian/Downloads/zig-macos-x86_64-0.11.0-dev.1499+23b7d2889/lib/std/start.zig:617:37              - _start
	//                           at /Users/adrian/Downloads/zig-macos-x86_64-0.11.0-dev.1499+23b7d2889/lib/std/start.zig:232:5
	//    2: wasm trap: wasm `unreachable` instruction executed
	for _, tc := range []struct {
		offset uint64
		exp    []string
	}{
		{offset: 0x7d - codeSecStart, exp: []string{"lib/std/builtin.zig:861:17"}},
		{offset: 0xa6 - codeSecStart, exp: []string{
			"main.zig:10:5 (inlined)",
			"main.zig:6:5 (inlined)",
			"main.zig:2:5",
		}},
		{offset: 0xb0 - codeSecStart, exp: []string{
			"lib/std/start.zig:617:37 (inlined)",
			"lib/std/start.zig:232:5",
		}},
	} {
		tc := tc
		t.Run(fmt.Sprintf("%#x/%s", tc.offset, tc.exp), func(t *testing.T) {
			// Ensures that DWARFLines.Line is goroutine-safe.
			hammer.NewHammer(t, 100, 5).Run(func(name string) {
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
	const codeSecStart = 0x000002c6

	// These cases are crafted by matching the stack trace result from wasmtime. To verify, run:
	//
	// 	WASMTIME_BACKTRACE_DETAILS=1 wasmtime run internal/testing/dwarftestdata/testdata/rust/main.wasm
	//
	// And this should produce the output as:
	// Caused by:
	// 	0: failed to invoke command default
	// 	1: error while executing at wasm backtrace:
	// 		0: 0x6385 - panic_abort::__rust_start_panic::abort::he70ae40a649be988
	// 						at /rustc/9eb3afe9ebe9c7d2b84b71002d44f4a0edac95e0/library/panic_abort/src/lib.rs:83:17              - __rust_start_panic
	// 						at /rustc/9eb3afe9ebe9c7d2b84b71002d44f4a0edac95e0/library/panic_abort/src/lib.rs:37:5
	// 		1: 0x6073 - rust_panic
	// 						at /rustc/9eb3afe9ebe9c7d2b84b71002d44f4a0edac95e0/library/std/src/panicking.rs:740:9
	// 		2: 0x603a - std::panicking::rust_panic_with_hook::h91fa5cbfc957c96c
	// 						at /rustc/9eb3afe9ebe9c7d2b84b71002d44f4a0edac95e0/library/std/src/panicking.rs:710:5
	// 		3: 0x50f4 - std::panicking::begin_panic_handler::{{closure}}::hea8cd6e90707c2a1
	// 						at /rustc/9eb3afe9ebe9c7d2b84b71002d44f4a0edac95e0/library/std/src/panicking.rs:577:13
	// 		4: 0x5056 - std::sys_common::backtrace::__rust_end_short_backtrace::h8766955a633f184a
	// 						at /rustc/9eb3afe9ebe9c7d2b84b71002d44f4a0edac95e0/library/std/src/sys_common/backtrace.rs:137:18
	// 		5: 0x56a8 - rust_begin_unwind
	// 						at /rustc/9eb3afe9ebe9c7d2b84b71002d44f4a0edac95e0/library/std/src/panicking.rs:575:5
	// 		6: 0xad7e - core::panicking::panic_fmt::hea96ca76090d462f
	// 						at /rustc/9eb3afe9ebe9c7d2b84b71002d44f4a0edac95e0/library/core/src/panicking.rs:64:14
	// 		7:  0x5cb - main::b::h47e5ee1ee8873d72
	// 						at /src/github.com/tetratelabs/wazero/internal/testing/dwarftestdata/testdata/rust/main.rs:12:5
	// 		8:  0x89c - main::a::h283f54b6c8e9f92b
	// 						at /src/github.com/tetratelabs/wazero/internal/testing/dwarftestdata/testdata/rust/main.rs:7:5              - main::main::hb80dac2ea6f04fda
	// 						at /src/github.com/tetratelabs/wazero/internal/testing/dwarftestdata/testdata/rust/main.rs:2:5
	// 		9:  0x408 - core::ops::function::FnOnce::call_once::hb25884e69c21780f
	// 						at /rustc/9eb3afe9ebe9c7d2b84b71002d44f4a0edac95e0/library/core/src/ops/function.rs:250:5
	// 		10:  0x877 - std::sys_common::backtrace::__rust_begin_short_backtrace::he6b53729c962b36c
	// 						at /rustc/9eb3afe9ebe9c7d2b84b71002d44f4a0edac95e0/library/std/src/sys_common/backtrace.rs:121:18
	// 		11:  0x69a - std::rt::lang_start::{{closure}}::hdfaa3c19d02e0d53
	// 						at /rustc/9eb3afe9ebe9c7d2b84b71002d44f4a0edac95e0/library/std/src/rt.rs:166:18
	// 		12: 0x289c - core::ops::function::impls::<impl core::ops::function::FnOnce<A> for &F>::call_once::hc4af877959b9a01b
	// 						at /rustc/9eb3afe9ebe9c7d2b84b71002d44f4a0edac95e0/library/core/src/ops/function.rs:287:13              - std::panicking::try::do_call::h6e3fca8ef3f0c311
	// 						at /rustc/9eb3afe9ebe9c7d2b84b71002d44f4a0edac95e0/library/std/src/panicking.rs:483:40              - std::panicking::try::hd3922896d41ddd64
	// 						at /rustc/9eb3afe9ebe9c7d2b84b71002d44f4a0edac95e0/library/std/src/panicking.rs:447:19              - std::panic::catch_unwind::h56bd273d658fbcdc
	// 						at /rustc/9eb3afe9ebe9c7d2b84b71002d44f4a0edac95e0/library/std/src/panic.rs:140:14              - std::rt::lang_start_internal::{{closure}}::h7aa6c5046d818502
	// 						at /rustc/9eb3afe9ebe9c7d2b84b71002d44f4a0edac95e0/library/std/src/rt.rs:148:48              - std::panicking::try::do_call::hfd6c12ed1cf59ae3
	// 						at /rustc/9eb3afe9ebe9c7d2b84b71002d44f4a0edac95e0/library/std/src/panicking.rs:483:40              - std::panicking::try::h40e1b077f288c786
	// 						at /rustc/9eb3afe9ebe9c7d2b84b71002d44f4a0edac95e0/library/std/src/panicking.rs:447:19              - std::panic::catch_unwind::h09dbc99d0be4be1f
	// 						at /rustc/9eb3afe9ebe9c7d2b84b71002d44f4a0edac95e0/library/std/src/panic.rs:140:14              - std::rt::lang_start_internal::h38aaea5d7881ae71
	// 						at /rustc/9eb3afe9ebe9c7d2b84b71002d44f4a0edac95e0/library/std/src/rt.rs:148:20
	// 		13:  0x637 - std::rt::lang_start::hb9ddebcd8dc7db51
	// 						at /rustc/9eb3afe9ebe9c7d2b84b71002d44f4a0edac95e0/library/std/src/rt.rs:165:17
	// 		14:  0x8c0 - <unknown>!__main_void
	// 		15:  0x2f2 - <unknown>!_start
	// 	2: wasm trap: wasm `unreachable` instruction executed

	for _, tc := range []struct {
		offset uint64
		exp    []string
	}{
		{offset: 0x6385 - codeSecStart, exp: []string{
			"/library/panic_abort/src/lib.rs:83:17",
			"/library/panic_abort/src/lib.rs:37:5",
		}},
		{offset: 0x6073 - codeSecStart, exp: []string{
			"/library/std/src/panicking.rs:740:9",
		}},
		{offset: 0x603a - codeSecStart, exp: []string{
			"/library/std/src/panicking.rs:710:5",
		}},
		{offset: 0x50f4 - codeSecStart, exp: []string{
			"/library/std/src/panicking.rs:577:13",
		}},
		{offset: 0x5056 - codeSecStart, exp: []string{
			"/library/std/src/sys_common/backtrace.rs:137:18",
		}},
		{offset: 0x56a8 - codeSecStart, exp: []string{
			"/library/std/src/panicking.rs:575:5",
		}},
		{offset: 0xad7e - codeSecStart, exp: []string{
			"/library/core/src/panicking.rs:64:14",
		}},
		{offset: 0x5cb - codeSecStart, exp: []string{
			"testdata/rust/main.rs:12:5",
		}},
		{offset: 0x89c - codeSecStart, exp: []string{
			"testdata/rust/main.rs:7:5",
			"testdata/rust/main.rs:2:5",
		}},
		{offset: 0x408 - codeSecStart, exp: []string{
			"/library/core/src/ops/function.rs:250:5",
		}},
		{offset: 0x877 - codeSecStart, exp: []string{
			"/library/std/src/sys_common/backtrace.rs:121:18",
		}},
		{offset: 0x69a - codeSecStart, exp: []string{
			"/library/std/src/rt.rs:166:18",
		}},
		{offset: 0x289c - codeSecStart, exp: []string{
			"/library/core/src/ops/function.rs:287:13",
			"/library/std/src/panicking.rs:483:40",
			"/library/std/src/panicking.rs:447:19",
			"/library/std/src/panic.rs:140:14",
			"/library/std/src/rt.rs:148:48",
			"/library/std/src/panicking.rs:483:40",
			"/library/std/src/panicking.rs:447:19",
			"/library/std/src/panic.rs:140:14",
			"/library/std/src/rt.rs:148:20",
		}},
		{offset: 0x637 - codeSecStart, exp: []string{
			"/library/std/src/rt.rs:165:17",
		}},
		{offset: 0x8c0 - codeSecStart, exp: []string{}},
		{offset: 0x2f2 - codeSecStart, exp: []string{}},
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
