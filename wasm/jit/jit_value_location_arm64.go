//go:build arm64
// +build arm64

package jit

import "github.com/twitchyliquid64/golang-asm/obj/arm64"

// Reserved registers.
const (
	// reservedRegisterForEngine R13: pointer to engine instance (i.e. *engine as uintptr)
	reservedRegisterForEngine = arm64.REG_R0
	// reservedRegisterForStackBasePointerAddress R14: stack base pointer's address (engine.stackBasePointer) in the current function call.
	reservedRegisterForStackBasePointerAddress = arm64.REG_R1
	// reservedRegisterForMemory R15: pointer to the memory slice's data (i.e. &memory.Buffer[0] as uintptr).
	reservedRegisterForMemory = arm64.REG_R2
)

var (
	generalPurposeFloatRegisters = []int16{
		arm64.REG_V0, arm64.REG_V1, arm64.REG_V2, arm64.REG_V3, arm64.REG_V4, arm64.REG_V5,
		arm64.REG_V6, arm64.REG_V7, arm64.REG_V8, arm64.REG_V9, arm64.REG_V10, arm64.REG_V11,
		arm64.REG_V12, arm64.REG_V13, arm64.REG_V14, arm64.REG_V15, arm64.REG_V16, arm64.REG_V17,
		arm64.REG_V18, arm64.REG_V19, arm64.REG_V20, arm64.REG_V21, arm64.REG_V22, arm64.REG_V23,
		arm64.REG_V24, arm64.REG_V25, arm64.REG_V26, arm64.REG_V27, arm64.REG_V28, arm64.REG_V29,
		arm64.REG_V30, arm64.REG_V31,
	}
	unreservedGeneralPurposeIntRegisters = []int16{
		arm64.REG_R3, arm64.REG_R4, arm64.REG_R5, arm64.REG_R6, arm64.REG_R7, arm64.REG_R8,
		arm64.REG_R9, arm64.REG_R10, arm64.REG_R11, arm64.REG_R12, arm64.REG_R13,
		arm64.REG_R14, arm64.REG_R15, arm64.REG_R16, arm64.REG_R17, arm64.REG_R18,
		arm64.REG_R19, arm64.REG_R20, arm64.REG_R21, arm64.REG_R22, arm64.REG_R23,
		arm64.REG_R24, arm64.REG_R25, arm64.REG_R26, arm64.REG_R27, arm64.REG_R28,
		arm64.REG_R29, arm64.REG_R30, arm64.REG_R31,
	}
)

func isIntRegister(r int16) bool {
	return unreservedGeneralPurposeIntRegisters[0] <= r && r <= unreservedGeneralPurposeIntRegisters[len(unreservedGeneralPurposeIntRegisters)-1]
}

func isFloatRegister(r int16) bool {
	return generalPurposeFloatRegisters[0] <= r && r <= generalPurposeFloatRegisters[len(generalPurposeFloatRegisters)-1]
}
