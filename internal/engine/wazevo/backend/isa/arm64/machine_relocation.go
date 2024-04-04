package arm64

import (
	"unsafe"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

// ResolveRelocations implements backend.Machine ResolveRelocations.
//
// TODO: unit test!
func (m *machine) ResolveRelocations(refToBinaryOffset map[ssa.FuncRef]int, binary []byte, relocations []backend.RelocationInfo) {
	base := uintptr(unsafe.Pointer(&binary[0]))
	for _, r := range relocations {
		instrOffset := r.Offset
		calleeFnOffset := refToBinaryOffset[r.FuncRef]
		brInstr := binary[instrOffset : instrOffset+4]
		diff := int64(calleeFnOffset) - (instrOffset)
		// Check if the diff is within the range of the branch instruction.
		if r.TrampolineOffset > 0 {
			// If the diff is out of range, we need to use a trampoline.
			diff = int64(r.TrampolineOffset) - instrOffset
			// The trampoline invokes the function using the BR instruction
			// using the absolute address of the callee function.
			// The BR instruction will not pollute LR, leaving set to the
			// PC at this location. Thus, upon return, the callee will
			// transparently return to the actual caller, skipping the trampoline.
			absoluteCalleeFnAddress := uint(base) + uint(calleeFnOffset)
			encodeTrampoline(absoluteCalleeFnAddress, binary, r.TrampolineOffset)
		}
		// https://developer.arm.com/documentation/ddi0596/2020-12/Base-Instructions/BL--Branch-with-Link-
		imm26 := diff / 4
		brInstr[0] = byte(imm26)
		brInstr[1] = byte(imm26 >> 8)
		brInstr[2] = byte(imm26 >> 16)
		if diff < 0 {
			brInstr[3] = (byte(imm26 >> 24 & 0b000000_01)) | 0b100101_10 // Set sign bit.
		} else {
			brInstr[3] = (byte(imm26 >> 24 & 0b000000_01)) | 0b100101_00 // No sign bit.
		}
	}
}

const relocationTrampolineSize = 5 * 4

func (m *machine) RelocationTrampolineSize(rels []backend.RelocationInfo) int {
	return relocationTrampolineSize * len(rels)
}

func (m *machine) UpdateRelocationInfo(refToBinaryOffset map[ssa.FuncRef]int, trampolineOffset int, r backend.RelocationInfo) (backend.RelocationInfo, int) {
	instrOffset := r.Offset
	calleeFnOffset := refToBinaryOffset[r.FuncRef]
	diff := int64(calleeFnOffset) - (instrOffset)
	trampolineSize := 0
	if diff < -(1<<25)*4 || diff > ((1<<25)-1)*4 {
		r.TrampolineOffset = trampolineOffset
		trampolineSize = relocationTrampolineSize
	}
	return r, trampolineSize
}

func encodeTrampoline(addr uint, binary []byte, instrOffset int) {
	// The tmpReg is safe to overwrite.
	tmpReg := regNumberInEncoding[tmp]

	const movzOp = uint32(0b10)
	const movkOp = uint32(0b11)
	// Note: for our purposes the 64-bit width of the reg should be enough.
	//       however, larger values could be written to and loaded from a constant pool (see encodeBrTableSequence).
	instrs := []uint32{
		encodeMoveWideImmediate(movzOp, tmpReg, uint64(uint16(addr)), 0, 1),
		encodeMoveWideImmediate(movkOp, tmpReg, uint64(uint16(addr>>16)), 1, 1),
		encodeMoveWideImmediate(movkOp, tmpReg, uint64(uint16(addr>>32)), 2, 1),
		encodeMoveWideImmediate(movkOp, tmpReg, uint64(uint16(addr>>48)), 3, 1),
		encodeUnconditionalBranchReg(tmpReg, false),
	}

	for i, inst := range instrs {
		instrBytes := binary[instrOffset+i*4 : instrOffset+(i+1)*4]
		instrBytes[0] = byte(inst)
		instrBytes[1] = byte(inst >> 8)
		instrBytes[2] = byte(inst >> 16)
		instrBytes[3] = byte(inst >> 24)
	}
}
