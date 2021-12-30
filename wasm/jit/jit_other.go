//go:build !amd64
// +build !amd64

package jit

import (
	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/wazeroir"
)

func jitcall(codeSegment, engine, memory uintptr) {
	panic("unsupported GOARCH")
}

func newCompiler(eng *engine, f *wasm.FunctionInstance, ir *wazeroir.CompilationResult) (compiler, error) {
	panic("unsupported GOARCH")
}
