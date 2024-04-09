package arm64

import (
	"encoding/binary"
	"math"
	"sort"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

const (
	trampolineCallSize = 4*4 + 4 // Four instructions + 32-bit immediate.

	// Unconditional branch offset is encoded as divided by 4 in imm26.
	// https://developer.arm.com/documentation/ddi0596/2021-12/Base-Instructions/BL--Branch-with-Link-?lang=en

	maxUnconditionalBranchOffset = maxSignedInt26 * 4
	minUnconditionalBranchOffset = minSignedInt26 * 4
)

// CallTrampolineIslandInfo implements backend.Machine CallTrampolineIslandInfo.
func (m *machine) CallTrampolineIslandInfo(numFunctions int) (interval, size int) {
	// Always place trampoline in the middle of the range.
	// TODO: precondition is that neither the trampoline size or a single function size is bigger than the range.
	//	Add asserts to check this.
	interval = int(maxUnconditionalBranchOffset) / 2
	size = trampolineCallSize * numFunctions
	return
}

// ResolveRelocations implements backend.Machine ResolveRelocations.
func (m *machine) ResolveRelocations(
	refToBinaryOffset map[ssa.FuncRef]int,
	executable []byte,
	relocations []backend.RelocationInfo,
	callTrampolineIslandOffsets []int,
) {
	for _, islandOffset := range callTrampolineIslandOffsets {
		encodeCallTrampolineIsland(refToBinaryOffset, islandOffset, executable)
	}

	for _, r := range relocations {
		instrOffset := r.Offset
		calleeFnOffset := refToBinaryOffset[r.FuncRef]
		diff := int64(calleeFnOffset) - (instrOffset)
		// Check if the diff is within the range of the branch instruction.
		if diff < minUnconditionalBranchOffset || diff > maxUnconditionalBranchOffset {
			// Find the nearest trampoline island from callTrampolineIslandOffsets.
			islandOffset := findNearestTrampolineIsland(callTrampolineIslandOffsets, int(instrOffset))
			islandTargetOffset := islandOffset + trampolineCallSize*int(r.FuncRef)
			diff = int64(islandTargetOffset) - (instrOffset)
			if diff < minUnconditionalBranchOffset || diff > maxUnconditionalBranchOffset {
				panic("BUG in trampoline placement")
			}
		}
		imm26 := diff / 4
		binary.LittleEndian.PutUint32(executable[instrOffset:instrOffset+4], encodeUnconditionalBranch(true, imm26))
	}
}

// encodeCallTrampolineIsland encodes a trampoline island for the given functions.
// Each island consists of a trampoline instruction sequence for each function.
// Each trampoline instruction sequence consists of 4 instructions + 32-bit immediate.
func encodeCallTrampolineIsland(refToBinaryOffset map[ssa.FuncRef]int, islandOffset int, executable []byte) {
	for i := 0; i < len(refToBinaryOffset); i++ {
		trampolineOffset := islandOffset + trampolineCallSize*i

		fnOffset := refToBinaryOffset[ssa.FuncRef(i)]
		diff := fnOffset - (trampolineOffset + 16)
		if diff > math.MaxInt32 || diff < math.MinInt32 {
			// This case even amd64 can't handle. 4GB is too big.
			panic("too big binary")
		}

		// The tmpReg, tmpReg2 is safe to overwrite (in fact any caller-saved register is safe to use).
		tmpReg, tmpReg2 := regNumberInEncoding[tmpRegVReg.RealReg()], regNumberInEncoding[x11]

		// adr tmpReg, PC+16: load the address of #diff into tmpReg.
		binary.LittleEndian.PutUint32(executable[trampolineOffset:], encodeAdr(tmpReg, 16))
		// ldrsw tmpReg2, [tmpReg]: Load #diff into tmpReg2.
		binary.LittleEndian.PutUint32(executable[trampolineOffset+4:],
			encodeLoadOrStore(sLoad32, tmpReg2, addressMode{kind: addressModeKindRegUnsignedImm12, rn: tmpRegVReg}))
		// add tmpReg, tmpReg2, tmpReg: add #diff to the address of #diff, getting the absolute address of the function.
		binary.LittleEndian.PutUint32(executable[trampolineOffset+8:],
			encodeAluRRR(aluOpAdd, tmpReg, tmpReg, tmpReg2, true, false))
		// br tmpReg: branch to the function without overwriting the link register.
		binary.LittleEndian.PutUint32(executable[trampolineOffset+12:], encodeUnconditionalBranchReg(tmpReg, false))
		// #diff
		binary.LittleEndian.PutUint32(executable[trampolineOffset+16:], uint32(diff))
	}
}

// findNearestTrampolineIsland finds the nearest trampoline island from callTrampolineIslandOffsets.
// precondition: callTrampolineIslandOffsets is sorted in ascending order.
func findNearestTrampolineIsland(callTrampolineIslandOffsets []int, offset int) int {
	// Find the nearest trampoline island from callTrampolineIslandOffsets.
	l := len(callTrampolineIslandOffsets)
	n := sort.Search(l, func(i int) bool {
		return callTrampolineIslandOffsets[i] >= offset
	})
	if n == l {
		n = l - 1
	}
	return callTrampolineIslandOffsets[n]
}
