//go:build arm64
// +build arm64

package jit

import (
	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/internal/wazeroir"
)

func jitcall(codeSegment, engine uintptr)

func newCompiler(f *wasm.FunctionInstance, ir *wazeroir.CompilationResult) (compiler, error) {
	panic("unsupported GOARCH")
}
