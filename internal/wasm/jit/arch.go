package jit

import (
	"runtime"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

var (
	// newArchContext returns a new archContext which is architecture-specific type to be embedded in callEngine.
	// This must be initialized in init() function in architecture-specific arch_*.go file which is guarded by build tag.
	newArchContext func() archContext
)

// jitcall is used by callEngine.execWasmFunction and the entrypoint to enter the JITed native code.
// codeSegment is the pointer to the initial instruction of the compiled native code.
// ce is "*callEngine" as uintptr.
//
// Note: this is implemented in per-arch Go assembler file. For example, arch_amd64.S implements this for amd64.
func jitcall(codeSegment, ce uintptr)

func init() {
	switch runtime.GOARCH {
	case "arm64":
		unreservedGeneralPurposeIntRegisters = arm64UnreservedGeneralPurposeIntRegisters
		unreservedGeneralPurposeFloatRegisters = arm64UnreservedGeneralPurposeFloatRegisters
	case "amd64":
		unreservedGeneralPurposeIntRegisters = amd64UnreservedGeneralPurposeIntRegisters
		unreservedGeneralPurposeFloatRegisters = amd64UnreservedGeneralPurposeFloatRegisters
	}
}

// newCompiler returns a new compiler interface which can be used to compile the given function instance.
// Note: ir param can be nil for host functions.
func newCompiler(f *wasm.FunctionInstance, ir *wazeroir.CompilationResult) (c compiler, err error) {
	switch runtime.GOARCH {
	case "arm64":
		c, err = newArm64Compiler(f, ir)
	case "amd64":
		c, err = newAmd64Compiler(f, ir)
	}
	return
}
