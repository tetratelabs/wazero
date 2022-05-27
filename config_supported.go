//go:build amd64 || arm64

package wazero

// CompilerSupported returns whether the compiler is supported in this environment.
const CompilerSupported = true

// NewRuntimeConfig returns NewRuntimeConfigCompiler
func NewRuntimeConfig() RuntimeConfig {
	return NewRuntimeConfigCompiler()
}
