package jit

import (
	"fmt"

	asm "github.com/twitchyliquid64/golang-asm"
	"github.com/twitchyliquid64/golang-asm/obj/x86"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func init() {
	jitcall = jitcallImpl
	newArchContext = newArchContextImpl
	newCompiler = newCompilerImpl
	unreservedGeneralPurposeFloatRegisters = []int16{
		x86.REG_X0, x86.REG_X1, x86.REG_X2, x86.REG_X3,
		x86.REG_X4, x86.REG_X5, x86.REG_X6, x86.REG_X7,
		x86.REG_X8, x86.REG_X9, x86.REG_X10, x86.REG_X11,
		x86.REG_X12, x86.REG_X13, x86.REG_X14, x86.REG_X15,
	}
	// Note that we never invoke "call" instruction,
	// so we don't need to care about the calling convention.
	// TODO: Maybe it is safe just save rbp, rsp somewhere
	// in Go-allocated variables, and reuse these registers
	// in JITed functions and write them back before returns.
	unreservedGeneralPurposeIntRegisters = []int16{
		x86.REG_AX, x86.REG_CX, x86.REG_DX, x86.REG_BX,
		x86.REG_SI, x86.REG_DI, x86.REG_R8, x86.REG_R9,
		x86.REG_R10, x86.REG_R11, x86.REG_R12,
	}
}

// jitcallImpl implements jitcall for amd64 architecture.
// Note: this function's body is defined in arch_amd64.s
func jitcallImpl(codeSegment, ce uintptr)

// newCompilerImpl implements newCompiler for amd64 architecture.
func newCompilerImpl(f *wasm.FunctionInstance, ir *wazeroir.CompilationResult) (compiler, error) {
	// We can choose arbitrary number instead of 1024 which indicates the cache size in the compiler.
	// TODO: optimize the number.
	b, err := asm.NewBuilder("amd64", 1024)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new assembly builder: %w", err)
	}

	compiler := &amd64Compiler{
		f:             f,
		builder:       b,
		locationStack: newValueLocationStack(),
		currentLabel:  wazeroir.EntrypointLabel,
		ir:            ir,
		labels:        map[string]*labelInfo{},
	}
	return compiler, nil
}

// archContext is embedded in callEngine in order to store architecture-specific data.
// For amd64, this is empty.
type archContext struct{}

// newArchContextImpl implements newArchContext for amd64 architecture.
func newArchContextImpl() (ret archContext) { return }
