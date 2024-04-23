package arm64

import (
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_maxNumFunctions(t *testing.T) {
	// Ensures that trampoline island is small enough to fit in the interval.
	require.True(t, maxNumFunctions*trampolineCallSize < trampolineIslandInterval/2) //nolint:gomnd
	// Ensures that each function is small enough to fit in the interval.
	require.True(t, maxFunctionExecutableSize < trampolineIslandInterval/2) //nolint:gomnd
}

func Test_encodeCallTrampolineIsland(t *testing.T) {
	executable := make([]byte, 16*1000)
	islandOffset := 160
	refToBinaryOffset := []int{0: 0, 1: 16, 2: 1600, 3: 16000}
	encodeCallTrampolineIsland(refToBinaryOffset, islandOffset, executable)
	for i := 0; i < len(refToBinaryOffset); i++ {
		offset := islandOffset + trampolineCallSize*i
		instrs := executable[offset : offset+trampolineCallSize-4]
		// Instructions are always the same except for the last immediate.
		require.Equal(t, "9b0000106b0380b97b030b8b60031fd6", hex.EncodeToString(instrs))
		imm := binary.LittleEndian.Uint32(executable[offset+trampolineCallSize-4:])
		require.Equal(t, uint32(refToBinaryOffset[ssa.FuncRef(i)]-(offset+16)), imm)
	}
}

func Test_searchTrampolineIsland(t *testing.T) {
	offsets := []int{16, 32, 48}
	require.Equal(t, 32, searchTrampolineIsland(offsets, 30))
	require.Equal(t, 32, searchTrampolineIsland(offsets, 17))
	require.Equal(t, 16, searchTrampolineIsland(offsets, 16))
	require.Equal(t, 48, searchTrampolineIsland(offsets, 48))
	require.Equal(t, 48, searchTrampolineIsland(offsets, 56))
	require.Equal(t, 16, searchTrampolineIsland(offsets, 1))
}
