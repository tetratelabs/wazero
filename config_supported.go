//go:build amd64 || arm64

package wazero

// NewRuntimeConfig returns NewRuntimeConfigCompiler
func NewRuntimeConfig() RuntimeConfig {
	return NewRuntimeConfigCompiler()
}
