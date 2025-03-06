package arm64

import (
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
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

	encodeCallTrampolineIsland(refToBinaryOffset, 0, islandOffset, executable)
	for i := 0; i < len(refToBinaryOffset); i++ {
		offset := islandOffset + trampolineCallSize*i
		instrs := executable[offset : offset+trampolineCallSize-4]
		// Instructions are always the same except for the last immediate.
		require.Equal(t, "9b0000106b0380b97b030b8b60031fd6", hex.EncodeToString(instrs))
		imm := binary.LittleEndian.Uint32(executable[offset+trampolineCallSize-4:])
		require.Equal(t, uint32(refToBinaryOffset[ssa.FuncRef(i)]-(offset+16)), imm)
	}

	executable = executable[:]
	// We test again, assuming that the first offset is an imported function.
	const importedFns = 1
	encodeCallTrampolineIsland(refToBinaryOffset, importedFns, islandOffset, executable)
	for i := 0; i < len(refToBinaryOffset[importedFns:]); i++ {
		offset := islandOffset + trampolineCallSize*i
		instrs := executable[offset : offset+trampolineCallSize-4]
		// Instructions are always the same except for the last immediate.
		require.Equal(t, "9b0000106b0380b97b030b8b60031fd6", hex.EncodeToString(instrs))
		imm := binary.LittleEndian.Uint32(executable[offset+trampolineCallSize-4:])
		require.Equal(t, uint32(refToBinaryOffset[ssa.FuncRef(i+importedFns)]-(offset+16)), imm)
	}
	// If the encoding is incorrect, then this offset should contain zeroes.
	finalOffset := islandOffset + trampolineCallSize*len(refToBinaryOffset)
	instrs := executable[finalOffset : finalOffset+trampolineCallSize]
	require.Equal(t, "0000000000000000000000000000000000000000", hex.EncodeToString(instrs))
}

func Test_ResolveRelocations_importedFns(t *testing.T) {
	tests := []struct {
		name          string
		refToOffset   []int
		importedFns   int
		relocations   []backend.RelocationInfo
		islandOffsets []int
	}{
		{
			name:        "Single imported function",
			refToOffset: []int{10, 20, 200 * 1024 * 1024},
			importedFns: 1,
			relocations: []backend.RelocationInfo{
				{Offset: 100, FuncRef: 2},
			},
			islandOffsets: []int{500},
		},
		{
			name:        "Multiple imported functions",
			refToOffset: []int{10, 20, 30, 40, 200 * 1024 * 1024},
			importedFns: 3,
			relocations: []backend.RelocationInfo{
				{Offset: 100, FuncRef: 4},
			},
			islandOffsets: []int{500},
		},
		{
			name:        "Multiple relocations with imported functions",
			refToOffset: []int{10, 20, 30, 40, 200 * 1024 * 1024, 250 * 1024 * 1024},
			importedFns: 2,
			relocations: []backend.RelocationInfo{
				{Offset: 100, FuncRef: 4},
				{Offset: 200, FuncRef: 5},
			},
			islandOffsets: []int{500},
		},
		{
			name:        "Many imported functions",
			refToOffset: []int{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 200 * 1024 * 1024},
			importedFns: 9,
			relocations: []backend.RelocationInfo{
				{Offset: 120, FuncRef: 10},
			},
			islandOffsets: []int{600},
		},
		{
			name:        "Multiple islands",
			refToOffset: []int{10, 20, 30, 40, 200 * 1024 * 1024, 300 * 1024 * 1024},
			importedFns: 3,
			relocations: []backend.RelocationInfo{
				{Offset: 100, FuncRef: 4},
				{Offset: 200*1024*1024 - 1000, FuncRef: 5},
			},
			islandOffsets: []int{500, 200*1024*1024 - 500},
		},
		{
			name:        "Multiple islands, no imported functions",
			refToOffset: []int{10, 20, 30, 40, 200 * 1024 * 1024, 300 * 1024 * 1024},
			importedFns: 0,
			relocations: []backend.RelocationInfo{
				{Offset: 100, FuncRef: 4},
				{Offset: 200*1024*1024 - 1000, FuncRef: 5},
			},
			islandOffsets: []int{500, 200*1024*1024 - 500},
		},
	}

	extractTargetInstruction := func(executable []byte, branchInstrOffset int64) uint32 {
		bytes := executable[branchInstrOffset : branchInstrOffset+4]
		return uint32(bytes[0]) | uint32(bytes[1])<<8 | uint32(bytes[2])<<16 | uint32(bytes[3])<<24
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create large executable buffer to handle far offsets
			executable := make([]byte, 310*1024*1024)

			m := &machine{}
			m.ResolveRelocations(
				tt.refToOffset,
				tt.importedFns,
				executable,
				tt.relocations,
				tt.islandOffsets,
			)

			for _, reloc := range tt.relocations {
				relocOffset := reloc.Offset

				// Extract the actual target instruction from the generated branch instruction
				actualInstruction := extractTargetInstruction(executable, relocOffset)

				// First find which island this relocation would use
				funcOffset := tt.refToOffset[reloc.FuncRef]
				diff := int64(funcOffset) - relocOffset

				// Only test relocations that need trampolines
				if diff < minUnconditionalBranchOffset || diff > maxUnconditionalBranchOffset {
					// Find the nearest trampoline island
					islandOffset := searchTrampolineIsland(tt.islandOffsets, int(relocOffset))

					expectedIslandOffset := islandOffset + trampolineCallSize*(int(reloc.FuncRef)-tt.importedFns)
					expectedBranchOffset := int64(expectedIslandOffset) - relocOffset
					expectedInstruction := encodeUnconditionalBranch(true, expectedBranchOffset)

					require.Equal(t, expectedInstruction, actualInstruction)

				} else {
					t.Logf("Skipping relocation that doesn't need a trampoline: %v", reloc)
				}
			}
		})
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
