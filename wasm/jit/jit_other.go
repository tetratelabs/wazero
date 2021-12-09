//go:build !amd64
// +build !amd64

package jit

import "github.com/tetratelabs/wazero/wasm"

func jitcall(codeSegment, engine, memory uintptr) {
	panic("unsupported GOARCH")
}

func (e *engine) compileWasmFunction(f *wasm.FunctionInstance) (*compiledWasmFunction, error) {
	panic("unsupported GOARCH")
}
