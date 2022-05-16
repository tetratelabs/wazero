//go:build amd64 || arm64

package wazero

const CompilerSupported = true

// NewRuntimeConfig returns NewRuntimeConfigCompiler
func NewRuntimeConfig() RuntimeConfig {
	return NewRuntimeConfigCompiler()
}
