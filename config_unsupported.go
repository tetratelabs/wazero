//go:build !amd64 && !arm64

package wazero

const CompilerSupported = false

// NewRuntimeConfig returns NewRuntimeConfigInterpreter
func NewRuntimeConfig() RuntimeConfig {
	return NewRuntimeConfigInterpreter()
}
