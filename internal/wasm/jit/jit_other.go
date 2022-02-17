//go:build !amd64 && !arm64
// +build !amd64,!arm64

package jit

import (
	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

type archContext struct{}

func newArchContext() (ret archContext) { return }

func jitcall(codeSegment, engine uintptr) {
	panic("unsupported GOARCH")
}

func newCompiler(f *wasm.FunctionInstance, ir *wazeroir.CompilationResult) (compiler, error) {
	panic("unsupported GOARCH")
}
