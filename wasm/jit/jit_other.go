//go:build !amd64 && !arm64
// +build !amd64,!arm64

package jit

import (
	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/internal/wazeroir"
)

type archContext {}

func jitcall(codeSegment, engine uintptr) {
	panic("unsupported GOARCH")
}

func newCompiler(f *wasm.FunctionInstance, ir *wazeroir.CompilationResult) (compiler, error) {
	panic("unsupported GOARCH")
}
