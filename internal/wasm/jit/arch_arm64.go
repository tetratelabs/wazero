package jit

import (
	"math"
)

// init initializes variables for the arm64 architecture
func init() {
	newArchContext = newArchContextImpl
}

// archContext is embedded in callEngine in order to store architecture-specific data.
type archContext struct {
	// jitCallReturnAddress holds the absolute return address for jitcall.
	// The value is set whenever jitcall is executed and done in jit_arm64.s
	// Native code can return back to the ce.execWasmFunction's main loop back by
	// executing "ret" instruction with this value. See arm64Compiler.exit.
	// Note: this is only used by JIT code so mark this as nolint.
	jitCallReturnAddress uint64 //nolint

	// Loading large constants in arm64 is a bit costly, so we place the following
	// consts on callEngine struct so that we can quickly access them during various operations.

	// minimum32BitSignedInt is used for overflow check for 32-bit signed division.
	// Note: this can be obtained by moving $1 and doing left-shift with 31, but it is
	// slower than directly loading fron this location.
	minimum32BitSignedInt int32
	// Note: this can be obtained by moving $1 and doing left-shift with 63, but it is
	// slower than directly loading fron this location.
	// minimum64BitSignedInt is used for overflow check for 64-bit signed division.
	minimum64BitSignedInt int64
}

const (
	// callEngineArchContextJITCallReturnAddressOffset is the offset of archContext.jitCallReturnAddress in callEngine.
	callEngineArchContextJITCallReturnAddressOffset = 120
	// callEngineArchContextMinimum32BitSignedIntOffset is the offset of archContext.minimum32BitSignedIntAddress in callEngine.
	callEngineArchContextMinimum32BitSignedIntOffset = 128
	// callEngineArchContextMinimum64BitSignedIntOffset is the offset of archContext.minimum64BitSignedIntAddress in callEngine.
	callEngineArchContextMinimum64BitSignedIntOffset = 136
)

// newArchContextImpl implements newArchContext for amd64 architecture.
func newArchContextImpl() archContext {
	return archContext{
		minimum32BitSignedInt: math.MinInt32,
		minimum64BitSignedInt: math.MinInt64,
	}
}
