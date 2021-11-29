//go:build !amd64
// +build !amd64

package jit

func jitcall(codeSegment, engine, memory uintptr) {
	panic("unsupported GOARCH")
}

func (e *engine) compile(*wasm.FunctionInstance) (*compiledWasmFunction, error) {
	panic("unsupported GOARCH")
}
