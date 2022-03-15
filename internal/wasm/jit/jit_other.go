//go:build !amd64 && !arm64

package jit

import (
	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

type archContext struct{}

func newArchContext() (ret archContext) { return }

func jitcall(codeSegment, ce uintptr) {
	panic("unsupported GOARCH")
}

func newCompiler(f *wasm.FunctionInstance, ir *wazeroir.CompilationResult) (compiler, func(), error) {
	panic("unsupported GOARCH")
}

const (
	reservedRegisterForCallEngine              = 0
	reservedRegisterForStackBasePointerAddress = 0
	reservedRegisterForMemory                  = 0
	reservedRegisterForTemporary               = 9
)

var (
	generalPurposeFloatRegisters         = []int16{}
	unreservedGeneralPurposeIntRegisters = []int16{}
)

const zeroRegister int16 = nilRegister
