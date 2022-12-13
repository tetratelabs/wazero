package wasmdebug_test

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/dwarftestdata"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
)

func TestDWARFLines_Line_TinyGo(t *testing.T) {
	mod, err := binary.DecodeModule(dwarftestdata.TinyGoWasm, api.CoreFeaturesV2, wasm.MemoryLimitPages, false, true, false)
	require.NoError(t, err)
	require.NotNil(t, mod.DWARFLines)

	// Get the offsets of functions named "a", "b" and "c" in dwarftestdata.TinyGoWasm.
	var a, b, c uint64
	for _, exp := range mod.ExportSection {
		switch exp.Name {
		case "a":
			a = mod.CodeSection[exp.Index-mod.ImportFuncCount()].BodyOffsetInCodeSection
		case "b":
			b = mod.CodeSection[exp.Index-mod.ImportFuncCount()].BodyOffsetInCodeSection
		case "c":
			c = mod.CodeSection[exp.Index-mod.ImportFuncCount()].BodyOffsetInCodeSection
		}
	}

	tests := []struct {
		name   string
		offset uint64
		exp    []string
	}{
		// Unknown offset returns empty string.
		{offset: math.MaxUint64},
		// The first instruction should point to the first line of each function in internal/testing/dwarftestdata/testdata/tinygo.go
		{offset: a, exp: []string{"wazero/internal/testing/dwarftestdata/testdata/tinygo/main.go:9:3"}},
		{offset: b, exp: []string{"wazero/internal/testing/dwarftestdata/testdata/tinygo/main.go:14:3"}},
		{offset: c, exp: []string{"wazero/internal/testing/dwarftestdata/testdata/tinygo/main.go:19:7"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Ensures that DWARFLines.Line is goroutine-safe.
			const concurrent = 100
			var wg sync.WaitGroup
			wg.Add(concurrent)

			for i := 0; i < concurrent; i++ {
				go func() {
					defer wg.Done()
					actual := mod.DWARFLines.Line(tc.offset)

					require.Equal(t, len(tc.exp), len(actual))
					for i := range tc.exp {
						require.Contains(t, actual[i], tc.exp[i])
					}
				}()
			}
			wg.Wait()
		})
	}
}

func TestDWARFLines_Line_Zig(t *testing.T) {
	mod, err := binary.DecodeModule(dwarftestdata.ZigWasm, api.CoreFeaturesV2, wasm.MemoryLimitPages, false, true, false)
	require.NoError(t, err)
	require.NotNil(t, mod.DWARFLines)

	// codeSecStart is the beginning of the first code entry in the Wasm binary.
	// If dwarftestdata.ZigWasm has been changed, we need to inspect by `wasm-tools dump`.
	const codeSecStart = 0x108
	// 0x12d-0x108

	// These cases are crafted by matching the stack trace result from wasmtime. To verify, run:
	//
	// 	WASMTIME_BACKTRACE_DETAILS=1 wasmtime run internal/testing/dwarftestdata/testdata/zig/main.wasm
	//
	// And this should produce the output as:
	//
	// Caused by:
	//    0: failed to invoke command default
	//    1: error while executing at wasm backtrace:
	//           0:  0x2bb - os.abort
	//                           at /Users/mathetake/zig-macos-aarch64-0.11.0-dev.618+096d3efae/lib/std/os.zig:552:9
	//           1:  0x18e - builtin.default_panic
	//                           at /Users/mathetake/zig-macos-aarch64-0.11.0-dev.618+096d3efae/lib/std/builtin.zig:787:25
	//           2:  0x12d - main.inlined_b
	//                           at ././main.zig:10:5              - main.inlined_a
	//                           at ././main.zig:6:5              - main.main
	//                           at ././main.zig:2:5
	//           3:  0x2ce - start.callMain
	//                           at /Users/mathetake/zig-macos-aarch64-0.11.0-dev.618+096d3efae/lib/std/start.zig:614:37              - _start
	//                           at /Users/mathetake/zig-macos-aarch64-0.11.0-dev.618+096d3efae/lib/std/start.zig:240:42
	//    2: wasm trap: wasm `unreachable` instruction executed
	for _, tc := range []struct {
		offset uint64
		exp    []string
	}{
		{offset: 0x2bb - codeSecStart, exp: []string{"lib/std/os.zig:552:9"}},
		{offset: 0x18e - codeSecStart, exp: []string{"lib/std/builtin.zig:787:25"}},
		{offset: 0x12d - codeSecStart, exp: []string{
			"main.zig:10:5 (inlined)",
			"main.zig:6:5 (inlined)",
			"main.zig:2:5",
		}},
		{offset: 0x2ce - codeSecStart, exp: []string{
			"lib/std/start.zig:614:37 (inlined)",
			"lib/std/start.zig:240:42",
		}},
	} {
		tc := tc
		t.Run(fmt.Sprintf("%#x/%s", tc.offset, tc.exp), func(t *testing.T) {
			actual := mod.DWARFLines.Line(tc.offset)

			t.Log(actual)

			require.Equal(t, len(tc.exp), len(actual))
			for i := range tc.exp {
				require.Contains(t, actual[i], tc.exp[i])
			}
		})
	}
}

func TestDWARFLines_Line_Rust(t *testing.T) {
	mod, err := binary.DecodeModule(dwarftestdata.RustWasm, api.CoreFeaturesV2, wasm.MemoryLimitPages, false, true, false)
	require.NoError(t, err)
	require.NotNil(t, mod.DWARFLines)

	const codeSecStart = 0x309

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

			fmt.Println(strings.Join(actual, "\n"))

			require.Equal(t, len(tc.exp), len(actual))
			for i := range tc.exp {
				require.Contains(t, actual[i], tc.exp[i])
			}
		})
	}
}
