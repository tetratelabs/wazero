//go:build arm64
// +build arm64

package jit

import "github.com/twitchyliquid64/golang-asm/obj/arm64"

// Reserved registers.
const (
	// reservedRegisterForCallEngine holds the pointer to callEngine instance (i.e. *virtulMachine as uintptr)
	reservedRegisterForCallEngine = arm64.REG_R0
	// reservedRegisterForStackBasePointerAddress holds stack base pointer's address (virtulMachine.stackBasePointer) in the current function call.
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
	// Note: arm64.REG_R29 is reserved for Goroutine by goruntime.
	unreservedGeneralPurposeIntRegisters = []int16{
		arm64.REG_R4, arm64.REG_R5, arm64.REG_R6, arm64.REG_R7, arm64.REG_R8,
		arm64.REG_R9, arm64.REG_R10, arm64.REG_R11, arm64.REG_R12, arm64.REG_R13,
		arm64.REG_R14, arm64.REG_R15, arm64.REG_R16, arm64.REG_R17, arm64.REG_R18,
		arm64.REG_R19, arm64.REG_R20, arm64.REG_R21, arm64.REG_R22, arm64.REG_R23,
		arm64.REG_R24, arm64.REG_R25, arm64.REG_R26, arm64.REG_R27, arm64.REG_R29,
		arm64.REG_R30,
	}
)

// simdRegisterForScalarFloatRegister returns SIMD register which corresponds to the given scalar float register.
// In other words, this returns: REG_F0 -> REG_V0, REG_F1 -> REG_V1, ...., REG_F31 -> REG_V31.
func simdRegisterForScalarFloatRegister(freg int16) int16 {
	return freg + (arm64.REG_F31 - arm64.REG_F0) + 1
}
