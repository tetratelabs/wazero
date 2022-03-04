//go:build amd64 || arm64

package wazero

const JITSupported = true

// NewRuntimeConfig returns NewRuntimeConfigJIT
func NewRuntimeConfig() *RuntimeConfig {
	return NewRuntimeConfigJIT()
}
