//go:build !amd64 && !arm64

package wazero

// NewRuntimeConfig returns NewRuntimeConfigInterpreter
func NewRuntimeConfig() RuntimeConfig {
	return NewRuntimeConfigInterpreter()
}
