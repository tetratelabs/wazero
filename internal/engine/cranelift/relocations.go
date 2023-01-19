package cranelift

import (
	"fmt"
	"runtime"
)

// TODO: doc
func applyFunctionRelocations(importedCount uint32, offsets []int, pends []pendingCompiledBody) {
	if runtime.GOARCH == "arm64" {
		applyFunctionRelocationsArm64(importedCount, offsets, pends)
	} else if runtime.GOARCH == "amd64" {
		applyFunctionRelocationsAmd64(importedCount, offsets, pends)
	} else {
		panic(fmt.Sprintf("BUG: unsupported GOARCH: %s", runtime.GOARCH))
	}
}

// applyFunctionRelocationsArm64 implements applyFunctionRelocations for arm64.
func applyFunctionRelocationsArm64(importedCount uint32, offsets []int, pends []pendingCompiledBody) {
	for i := range pends {
		pend := &pends[i]
		for _, reloc := range pend.relocs {
			instrOffset := reloc.offset
			calleeFnOffset := offsets[reloc.index-importedCount]
			brInstr := pend.machineCode[instrOffset : instrOffset+4]
			diff := int64(calleeFnOffset) - (int64(instrOffset) + int64(offsets[i]))
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
}

// applyFunctionRelocationsAmd64 implements applyFunctionRelocations for amd64.
func applyFunctionRelocationsAmd64(importedCount uint32, offsets []int, pends []pendingCompiledBody) {
	panic("TODO")
}
