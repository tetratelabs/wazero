//go:build amd64 || arm64
// +build amd64 arm64

package wazero

// NewRuntimeConfig returns NewRuntimeConfigInterpreter
// TODO: switch back to NewRuntimeConfigJIT https://github.com/tetratelabs/wazero/issues/308
func NewRuntimeConfig() *RuntimeConfig {
	return NewRuntimeConfigInterpreter()
}
