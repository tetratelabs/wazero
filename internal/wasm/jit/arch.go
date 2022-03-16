package jit

import (
	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

// Following variables must be initialized in init() function in architecture-specific arch_*.go file which
// is guarded by build tag. This file exists as build-tag free in order to provide documentation interface
// over these per-architecture definitions of the following symbols.
var (
	// newArchContext returns a new archContext which is architecture-specific type to be embedded in callEngine.
	newArchContext func() archContext

	// jitcall is used by callEngine.execWasmFunction and the entrypoint to enter the JITed native code.
	// codeSegment is the pointer to the initial instruction of the compiled native code.
	// ce is "*callEngine" as uintptr.
	jitcall func(codeSegment, ce uintptr)

	// newCompiler returns a new compiler interface which can be used to compile the given function instance.
	// The function returned must be invoked when finished compiling, so use `defer` to ensure this.
	// Note: ir param can be nil for host functions.
	newCompiler func(f *wasm.FunctionInstance, ir *wazeroir.CompilationResult) (compiler, error)

	// unreservedGeneralPurposeIntRegisters contains unreserved general purpose registers of integer type.
	unreservedGeneralPurposeIntRegisters []int16

	// unreservedGeneralPurposeFloatRegisters contains unreserved general purpose registers of scalar float type.
	unreservedGeneralPurposeFloatRegisters []int16
)
