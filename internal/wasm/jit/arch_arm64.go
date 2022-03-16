package jit

import (
	"fmt"
	"math"

	"github.com/twitchyliquid64/golang-asm/obj/arm64"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

// init initializes variables for the arm64 architecture
func init() {
	jitcall = jitcallImpl
	newCompiler = newCompilerImpl
	newArchContext = newArchContextImpl
	unreservedGeneralPurposeFloatRegisters = []int16{
		arm64.REG_F0, arm64.REG_F1, arm64.REG_F2, arm64.REG_F3,
		arm64.REG_F4, arm64.REG_F5, arm64.REG_F6, arm64.REG_F7, arm64.REG_F8,
		arm64.REG_F9, arm64.REG_F10, arm64.REG_F11, arm64.REG_F12, arm64.REG_F13,
		arm64.REG_F14, arm64.REG_F15, arm64.REG_F16, arm64.REG_F17, arm64.REG_F18,
		arm64.REG_F19, arm64.REG_F20, arm64.REG_F21, arm64.REG_F22, arm64.REG_F23,
		arm64.REG_F24, arm64.REG_F25, arm64.REG_F26, arm64.REG_F27, arm64.REG_F28,
		arm64.REG_F29, arm64.REG_F30, arm64.REG_F31,
	}
	// Note (see arm64 section in https://go.dev/doc/asm):
	// * REG_R18 is reserved as a platform register, and we don't use it in JIT.
	// * REG_R28 is reserved for Goroutine by Go runtime, and we don't use it in JIT.
	unreservedGeneralPurposeIntRegisters = []int16{
		arm64.REG_R4, arm64.REG_R5, arm64.REG_R6, arm64.REG_R7, arm64.REG_R8,
		arm64.REG_R9, arm64.REG_R10, arm64.REG_R11, arm64.REG_R12, arm64.REG_R13,
		arm64.REG_R14, arm64.REG_R15, arm64.REG_R16, arm64.REG_R17, arm64.REG_R19,
		arm64.REG_R20, arm64.REG_R21, arm64.REG_R22, arm64.REG_R23, arm64.REG_R24,
		arm64.REG_R25, arm64.REG_R26, arm64.REG_R27, arm64.REG_R29, arm64.REG_R30,
	}
}

// jitcallImpl implements jitcallfor arm64 architecture.
// Note: this function's body is defined in arch_arm64.s
func jitcallImpl(codeSegment, ce uintptr)

// newCompilerImpl implements newCompiler for amd64 architecture.
func newCompilerImpl(f *wasm.FunctionInstance, ir *wazeroir.CompilationResult) (compiler, error) {
	b, err := asm.NewBuilder("arm64", 1024)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new assembly builder: %w", err)
	}

	compiler := &arm64Compiler{
		f:             f,
		builder:       b,
		locationStack: newValueLocationStack(),
		ir:            ir,
		labels:        map[string]*labelInfo{},
	}
	return compiler, nil
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
