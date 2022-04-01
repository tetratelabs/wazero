package jit

import (
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

// init initializes variables for the amd64 architecture
func init() {
	newArchContext = newArchContextImpl
}

// archContext is embedded in callEngine in order to store architecture-specific data.
// For amd64, this is empty.
type archContext struct{}

// newArchContextImpl implements newArchContext for amd64 architecture.
func newArchContextImpl() (ret archContext) { return }

func init() {
	unreservedGeneralPurposeIntRegisters = amd64UnreservedGeneralPurposeIntRegisters
	unreservedGeneralPurposeFloatRegisters = amd64UnreservedGeneralPurposeFloatRegisters
}

// newCompiler returns a new compiler interface which can be used to compile the given function instance.
// Note: ir param can be nil for host functions.
func newCompiler(f *wasm.FunctionInstance, ir *wazeroir.CompilationResult) (compiler, error) {
	return newAmd64Compiler(f, ir)
}
