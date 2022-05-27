//go:build !amd64 && !arm64

package wazero

// CompilerSupported returns whether the compiler is supported in this environment.
const CompilerSupported = false

// NewRuntimeConfig returns NewRuntimeConfigInterpreter
func NewRuntimeConfig() RuntimeConfig {
	return NewRuntimeConfigInterpreter()
}
