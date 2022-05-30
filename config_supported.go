//go:build amd64 || arm64

package wazero

// This file's implementation of the below function always returns compiler, but the doc for it should
// describe the behavior of the API when the user calls it, not the implementation in this file which
// may not be used based on build tags.

// NewRuntimeConfig returns a RuntimeConfig using the compiler if it is supported in this environment,
// or the interpreter otherwise.
func NewRuntimeConfig() RuntimeConfig {
	return NewRuntimeConfigCompiler()
}
