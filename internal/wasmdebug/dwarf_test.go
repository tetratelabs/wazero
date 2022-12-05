package wasmdebug_test

import (
	"math"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/dwarftestdata"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
	"github.com/tetratelabs/wazero/internal/wasmdebug"
)

func TestGetSourceInfo(t *testing.T) {
	mod, err := binary.DecodeModule(dwarftestdata.DWARFWasm, api.CoreFeaturesV2, wasm.MemoryLimitPages, false, true, false)
	require.NoError(t, err)
	require.NotNil(t, mod.DWARF)

	// Get the offsets of functions named "a", "b" and "c" in dwarftestdata.DWARFWasm.
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
		offset uint64
		exp    string
	}{
		// Unknown offset returns empty string.
		{offset: math.MaxUint64, exp: ""},
		// The first instruction should point to the first line of each function in internal/testing/dwarftestdata/testdata/main.go
		{offset: a, exp: "wazero/internal/testing/dwarftestdata/testdata/main.go:9:3"},
		{offset: b, exp: "wazero/internal/testing/dwarftestdata/testdata/main.go:14:3"},
		{offset: c, exp: "wazero/internal/testing/dwarftestdata/testdata/main.go:19:7"},
	}

	for _, tc := range tests {
		t.Run(tc.exp, func(t *testing.T) {
			actual := wasmdebug.GetSourceInfo(mod.DWARF, tc.offset)
			require.Contains(t, actual, tc.exp)
		})
	}
}
