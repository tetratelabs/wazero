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
	//                           at /Users/adrian/Downloads/zig-macos-x86_64-0.11.0-dev.1499+23b7d2889/lib/std/builtin.zig:858:17
	//           1:   0xa6 - main.inlined_b
	//                           at /Users/adrian/oss/wazero/internal/testing/dwarftestdata/testdata/zig/main.zig:10:5              - main.inlined_a
	//                           at /Users/adrian/oss/wazero/internal/testing/dwarftestdata/testdata/zig/main.zig:6:5              - main.main
	//                           at /Users/adrian/oss/wazero/internal/testing/dwarftestdata/testdata/zig/main.zig:2:5
	//           2:   0xb0 - start.callMain
	//                           at /Users/adrian/Downloads/zig-macos-x86_64-0.11.0-dev.1499+23b7d2889/lib/std/start.zig:616:37              - _start
	//                           at /Users/adrian/Downloads/zig-macos-x86_64-0.11.0-dev.1499+23b7d2889/lib/std/start.zig:232:5
	//    2: wasm trap: wasm `unreachable` instruction executed
	for _, tc := range []struct {
		offset uint64
		exp    []string
	}{
		{offset: 0x7d - codeSecStart, exp: []string{"lib/std/builtin.zig:858:17"}},
		{offset: 0xa6 - codeSecStart, exp: []string{
			"main.zig:10:5 (inlined)",
			"main.zig:6:5 (inlined)",
			"main.zig:2:5",
		}},
		{offset: 0xb0 - codeSecStart, exp: []string{
			"lib/std/start.zig:616:37 (inlined)",
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
			if t.Failed() {
				return // At least one test failed, so return now.
			}
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
	const codeSecStart = 0x309

	// These cases are crafted by matching the stack trace result from wasmtime. To verify, run:
	//
	// 	WASMTIME_BACKTRACE_DETAILS=1 wasmtime run internal/testing/dwarftestdata/testdata/rust/main.wasm
	//
	// And this should produce the output as:
	// Caused by:
	//    0: failed to invoke command default
	//    1: error while executing at wasm backtrace:
	//           0: 0xc77d - core::ptr::const_ptr::<impl *const T>::offset::ha55096d7e14d75d8
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/slice/index.rs:286:39              - core::ptr::const_ptr::<impl *const T>::add::h089d5a72f68a4291
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/ptr/const_ptr.rs:870:18              - <core::ops::range::Range<usize> as core::slice::index::SliceIndex<[T]>>::get_unchecked::h29ddcf1882fa0f66
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/slice/index.rs:286:39              - <core::ops::range::RangeFrom<usize> as core::slice::index::SliceIndex<[T]>>::get_unchecked::h75ebc890f16858ff
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/slice/mod.rs:1630:46              - core::slice::<impl [T]>::get_unchecked::h6278e5a065ea078a
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/slice/mod.rs:405:20              - core::slice::<impl [T]>::split_at_unchecked::h88a6f1e7c576a79c
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/slice/mod.rs:1630:46              - core::slice::<impl [T]>::split_at::h68e6904057100aef
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/slice/mod.rs:1548:18              - <core::slice::iter::Chunks<T> as core::iter::traits::iterator::Iterator>::next::h9e3ea1e50ad1cfcf
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/slice/iter.rs:1478:30              - core::str::count::do_count_chars::h124622240ac1fb8b
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/str/count.rs:74:18
	//           1: 0xa701 - core::str::validations::utf8_acc_cont_byte::hb47c34b8c4cbf06b
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/str/validations.rs:57:19              - core::str::validations::next_code_point::hbb42fe8b8fcbddc3
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/str/validations.rs:57:19              - <core::str::iter::Chars as core::iter::traits::iterator::Iterator>::next::h2dc4678e3c0bda18
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/str/iter.rs:140:15              - <core::str::iter::CharIndices as core::iter::traits::iterator::Iterator>::next::h430b9a1b0d2fcfcd
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/str/iter.rs:140:15              - core::iter::traits::iterator::Iterator::advance_by::hadbf2e62b9ea873e
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/iter/traits/iterator.rs:330:13              - core::iter::traits::iterator::Iterator::nth::h68978ac344a2c26f
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/iter/traits/iterator.rs:377:9              - core::fmt::Formatter::pad::hc91f9fb3fb51f81f
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/fmt/mod.rs:1455:35
	//           2: 0x45e8 - alloc::alloc::dealloc::hde3d57428722ee9b
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/alloc/src/alloc.rs:244:22              - <alloc::alloc::Global as core::alloc::Allocator>::deallocate::h9c672f23742d6fbc
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/alloc/src/alloc.rs:244:22              - alloc::alloc::box_free::hd090040c59659308
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/alloc/src/alloc.rs:342:9              - core::ptr::drop_in_place<alloc::boxed::Box<std::io::error::Custom>>::h3d2c76e2b4a26668
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/ptr/mod.rs:487:1              - core::ptr::drop_in_place<std::io::error::ErrorData<alloc::boxed::Box<std::io::error::Custom>>>::hcaa143fc963fdc85
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/ptr/mod.rs:487:1              - core::ptr::drop_in_place<std::io::error::repr_unpacked::Repr>::hf8eda15dbd953cd1
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/ptr/mod.rs:487:1              - core::ptr::drop_in_place<std::io::error::Error>::ha50d906acd95a768
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/ptr/mod.rs:487:1              - core::ptr::drop_in_place<core::result::Result<(),std::io::error::Error>>::h1a246d5cbc0481cf
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/ptr/mod.rs:487:1              - core::mem::drop::h37b541d3c993930c
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/std/src/panicking.rs:292:17              - std::panicking::default_hook::{{closure}}::h78d75d30689791e7
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/std/src/panicking.rs:292:17
	//           3: 0xad95 - core::fmt::ArgumentV1::as_usize::h1da6b057d1a7dc54
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/fmt/mod.rs:362:12              - core::fmt::getcount::h8c5d6b3aea75a2d3
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/fmt/mod.rs:1257:22              - core::fmt::run::h78a98448d78ecec3
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/fmt/mod.rs:1235:21              - core::fmt::write::h5471a2341ce22f17
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/fmt/mod.rs:1214:26
	//           4: 0xc44d - core::fmt::Write::write_fmt::h4a7e084f8beacf08
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/fmt/mod.rs:188:26
	//           5: 0xbae8 - core::fmt::Formatter::write_str::hc634aaecc183d175
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/fmt/mod.rs:1629:9              - core::fmt::builders::DebugStruct::finish_non_exhaustive::{{closure}}::h51dc89dce87b7120
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/fmt/builders.rs:199:17              - core::result::Result<T,E>::and_then::hea34a5d4dd616ad6
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/result.rs:1352:22              - core::fmt::builders::DebugStruct::finish_non_exhaustive::h87daf5524c71dda9
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/fmt/builders.rs:187:23
	//           6: 0xc046 - core::fmt::Formatter::pad_integral::ha8bb3db77298fecc
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/fmt/mod.rs:1384:17
	//           7: 0xb035 - core::slice::memchr::memchr_general_case::hb481b2edf3b1871e
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/slice/memchr.rs
	//           8: 0xc06a - core::fmt::Formatter::pad_integral::ha8bb3db77298fecc
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/fmt/mod.rs
	//           9: 0xc09e - core::fmt::Formatter::padding::h4b882ffb39d00a12
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/fmt/mod.rs:1504:35              - core::fmt::Formatter::pad_integral::ha8bb3db77298fecc
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/fmt/mod.rs:1407:36
	//          10: 0xb1a3 - <core::panic::location::Location as core::fmt::Display>::fmt::hf3870c0af6a67fac
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/panic/location.rs:196:6
	//          11: 0x8ee0 - <unknown>!std::rt::lang_start_internal::h3c39e5d3c278a90f
	//          12: 0xae9d - core::fmt::write::h5471a2341ce22f17
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/fmt/mod.rs:1226:2
	//          13: 0xc3b7 - core::char::methods::encode_utf8_raw::h700e7a293d6eb2b7
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/char/methods.rs:1677:13              - core::char::methods::<impl char>::encode_utf8::h641f2d1d001008d5
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/fmt/mod.rs:165:24              - core::fmt::Write::write_char::ha951e2975b9730c3
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/fmt/mod.rs:165:24
	//          14: 0xc418 - core::fmt::Write::write_fmt::h4a7e084f8beacf08
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/fmt/mod.rs:187
	//          15: 0xc6e7 - core::iter::traits::iterator::Iterator::fold::h99ed29c108afc948
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/iter/traits/iterator.rs:2414:21              - <core::iter::adapters::map::Map<I,F> as core::iter::traits::iterator::Iterator>::fold::ha5be3bb1eeeaf8fe
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/iter/adapters/map.rs:124:9              - <usize as core::iter::traits::accum::Sum>::sum::h00b4d0c0300e94a9
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/iter/traits/accum.rs:42:17              - core::iter::traits::iterator::Iterator::sum::h04374b17d4abbea5
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/iter/traits/iterator.rs:3347:9              - <core::iter::adapters::filter::Filter<I,P> as core::iter::traits::iterator::Iterator>::count::h5c8b4c5e67e6831c
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/str/count.rs:135:5              - core::str::count::char_count_general_case::hfffa06842344b9fe
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/str/count.rs:135:5              - core::str::count::do_count_chars::h124622240ac1fb8b
	//                           at /rustc/c396bb3b8a16b1f2762b7c6078dc3e023f6a2493/library/core/src/str/count.rs:71:21
	for _, tc := range []struct {
		offset uint64
		exp    []string
	}{
		{offset: 0xc77d - codeSecStart, exp: []string{
			"/library/core/src/slice/index.rs:286:39",
			"/library/core/src/ptr/const_ptr.rs:870:18",
			"/library/core/src/slice/index.rs:286:39",
			"/library/core/src/slice/mod.rs:1630:46",
			"/library/core/src/slice/mod.rs:405:20",
			"/library/core/src/slice/mod.rs:1630:46",
			"/library/core/src/slice/mod.rs:1548:18",
			"/library/core/src/slice/iter.rs:1478:30",
			"/library/core/src/str/count.rs:74:18",
		}},
		{offset: 0xc06a - codeSecStart, exp: []string{"/library/core/src/fmt/mod.rs"}},
		{offset: 0xc6e7 - codeSecStart, exp: []string{
			"/library/core/src/iter/traits/iterator.rs:2414:21",
			"/library/core/src/iter/adapters/map.rs:124:9",
			"/library/core/src/iter/traits/accum.rs:42:17",
			"/library/core/src/iter/traits/iterator.rs:3347:9",
			"/library/core/src/str/count.rs:135:5",
			"/library/core/src/str/count.rs:135:5",
			"/library/core/src/str/count.rs:71:21",
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
