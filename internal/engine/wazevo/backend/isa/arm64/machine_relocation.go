package arm64

import (
	"encoding/binary"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

// ResolveRelocations implements backend.Machine ResolveRelocations.
func (m *machine) ResolveRelocations(refToBinaryOffset map[ssa.FuncRef]int, binary []byte, relocations []backend.RelocationInfo) {
	base := uintptr(unsafe.Pointer(&binary[0]))
	for _, r := range relocations {
		instrOffset := r.Offset
		calleeFnOffset := refToBinaryOffset[r.FuncRef]
		brInstr := binary[instrOffset : instrOffset+4]
		diff := int64(calleeFnOffset) - (instrOffset)
		// The diff was outside the range of the branch instruction, so we use a
		// trampoline (see UpdateRelocationInfo).
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

func (m *machine) RelocationTrampolineSize(rels []backend.RelocationInfo) (trampolineSize int) {
	for _, rel := range rels {
		if rel.TrampolineOffset > 0 {
			trampolineSize += relocationTrampolineSize
		}
	}
	return
}

func (m *machine) UpdateRelocationInfo(refToBinaryOffset map[ssa.FuncRef]int, trampolineOffset int, r *backend.RelocationInfo) int {
	instrOffset := r.Offset
	calleeFnOffset := refToBinaryOffset[r.FuncRef]
	// BR immediate is 26-bit, shifted right by 2.
	// So we compute the diff between the (estimated) offset of the target function
	// and the offset of the callee and we check that the result is within range.
	diff := int64(calleeFnOffset) - (instrOffset)
	trampolineSize := 0
	if diff < minSignedInt26*4 || diff > maxSignedInt26*4 {
		// If it is _not_ within range, then we assign the TrampolineOffset
		// which will be nonzero for all the cases where a trampoline is actually needed.
		r.TrampolineOffset = trampolineOffset
		// Currently the trampoline size is fixed.
		trampolineSize = relocationTrampolineSize
	}
	return trampolineSize
}

func encodeTrampoline(addr uint, bytes []byte, instrOffset int) {
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
		instrBytes := bytes[instrOffset+i*4 : instrOffset+(i+1)*4]
		binary.LittleEndian.PutUint32(instrBytes, inst)
	}
}
