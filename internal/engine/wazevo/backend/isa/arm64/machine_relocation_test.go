package arm64

import (
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_encodeCallTrampolineIsland(t *testing.T) {
	executable := make([]byte, 16*1000)
	islandOffset := 160
	refToBinaryOffset := map[ssa.FuncRef]int{0: 0, 1: 16, 2: 1600, 3: 16000}
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
