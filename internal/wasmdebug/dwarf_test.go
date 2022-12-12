package wasmdebug_test

import (
	"fmt"
	"math"
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
		{offset: a, exp: []string{"wazero/internal/testing/dwarftestdata/testdata/main.go:9:3"}},
		{offset: b, exp: []string{"wazero/internal/testing/dwarftestdata/testdata/main.go:14:3"}},
		{offset: c, exp: []string{"wazero/internal/testing/dwarftestdata/testdata/main.go:19:7"}},
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

	// These cases are crafted by matching the stack trace result from wasmtime. To verify, run:
	//
	// 	WASMTIME_BACKTRACE_DETAILS=1 wasmtime run internal/testing/dwarftestdata/testdata/zig.wasm
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
