package jit

import (
	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/jit/asm"
	"github.com/tetratelabs/wazero/internal/wasm/jit/asm/amd64"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

// init initializes variables for the amd64 architecture
func init() {
	jitcall = jitcallImpl
	newCompiler = newCompilerImpl
	newArchContext = newArchContextImpl
	unreservedGeneralPurposeFloatRegisters = []asm.Register{
		amd64.REG_X0, amd64.REG_X1, amd64.REG_X2, amd64.REG_X3,
		amd64.REG_X4, amd64.REG_X5, amd64.REG_X6, amd64.REG_X7,
		amd64.REG_X8, amd64.REG_X9, amd64.REG_X10, amd64.REG_X11,
		amd64.REG_X12, amd64.REG_X13, amd64.REG_X14, amd64.REG_X15,
	}
	// Note that we never invoke "call" instruction,
	// so we don't need to care about the calling convention.
	// TODO: Maybe it is safe just save rbp, rsp somewhere
	// in Go-allocated variables, and reuse these registers
	// in JITed functions and write them back before returns.
	unreservedGeneralPurposeIntRegisters = []asm.Register{
		amd64.REG_AX, amd64.REG_CX, amd64.REG_DX, amd64.REG_BX,
		amd64.REG_SI, amd64.REG_DI, amd64.REG_R8, amd64.REG_R9,
		amd64.REG_R10, amd64.REG_R11, amd64.REG_R12,
	}
}

// jitcallImpl implements jitcall for amd64 architecture.
// Note: this function's body is defined in arch_amd64.s
func jitcallImpl(codeSegment, ce uintptr)

// newCompilerImpl implements newCompiler for amd64 architecture.
func newCompilerImpl(f *wasm.FunctionInstance, ir *wazeroir.CompilationResult) (compiler, error) {
	b, err := amd64.NewAssembler()
	if err != nil {
		return nil, err
	}
	compiler := &amd64Compiler{
		f:             f,
		assembler:     b,
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
