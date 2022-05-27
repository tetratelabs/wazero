//go:build !amd64 && !arm64

package wazero

const CompilerSupported = false

// NewRuntimeConfig returns a RuntimeConfig using the compiler if it is supported in this
// environment, or interpreter otherwise.
func NewRuntimeConfig() RuntimeConfig {
	return NewRuntimeConfigInterpreter()
}
