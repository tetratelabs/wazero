//go:build arm64
// +build arm64

package jit

import "github.com/twitchyliquid64/golang-asm/obj/arm64"

// Reserved registers.
const (
	// reservedRegisterForEngine holds the pointer to engine instance (i.e. *engine as uintptr)
	reservedRegisterForEngine = arm64.REG_R0
	// reservedRegisterForStackBasePointerAddress holds stack base pointer's address (engine.stackBasePointer) in the current function call.
	reservedRegisterForStackBasePointerAddress = arm64.REG_R1
	// reservedRegisterForMemory holds the pointer to the memory slice's data (i.e. &memory.Buffer[0] as uintptr).
	reservedRegisterForMemory    = arm64.REG_R2
	reservedRegisterForTemporary = arm64.REG_R3
)

// zeroRegister is the alias of the arm64-specific zero register for readability.
const zeroRegister int16 = arm64.REGZERO

var (
	generalPurposeFloatRegisters = []int16{
		arm64.REG_F0, arm64.REG_F1, arm64.REG_F2, arm64.REG_F3,
		arm64.REG_F4, arm64.REG_F5, arm64.REG_F6, arm64.REG_F7, arm64.REG_F8,
		arm64.REG_F9, arm64.REG_F10, arm64.REG_F11, arm64.REG_F12, arm64.REG_F13,
		arm64.REG_F14, arm64.REG_F15, arm64.REG_F16, arm64.REG_F17, arm64.REG_F18,
		arm64.REG_F19, arm64.REG_F20, arm64.REG_F21, arm64.REG_F22, arm64.REG_F23,
		arm64.REG_F24, arm64.REG_F25, arm64.REG_F26, arm64.REG_F27, arm64.REG_F28,
		arm64.REG_F29, arm64.REG_F30, arm64.REG_F31,
	}
	unreservedGeneralPurposeIntRegisters = []int16{
		arm64.REG_R4, arm64.REG_R5, arm64.REG_R6, arm64.REG_R7, arm64.REG_R8,
		arm64.REG_R9, arm64.REG_R10, arm64.REG_R11, arm64.REG_R12, arm64.REG_R13,
		arm64.REG_R14, arm64.REG_R15, arm64.REG_R16, arm64.REG_R17, arm64.REG_R18,
		arm64.REG_R19, arm64.REG_R20, arm64.REG_R21, arm64.REG_R22, arm64.REG_R23,
		arm64.REG_R24, arm64.REG_R25, arm64.REG_R26, arm64.REG_R27, arm64.REG_R28,
		arm64.REG_R29, arm64.REG_R30,
	}
)

func simdRegisterForScalarFloatRegister(fn int16) (vn int16) {
	return fn + (arm64.REG_F31 - arm64.REG_F0) + 1
}

func isIntRegister(r int16) bool {
	return unreservedGeneralPurposeIntRegisters[0] <= r && r <= unreservedGeneralPurposeIntRegisters[len(unreservedGeneralPurposeIntRegisters)-1]
}

func isFloatRegister(r int16) bool {
	return generalPurposeFloatRegisters[0] <= r && r <= generalPurposeFloatRegisters[len(generalPurposeFloatRegisters)-1]
}

func isZeroRegister(r int16) bool {
	return r == zeroRegister
}
