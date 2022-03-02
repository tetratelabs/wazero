//go:build amd64 || arm64
// +build amd64 arm64

package wazero

// NewRuntimeConfig returns NewRuntimeConfigJIT
func NewRuntimeConfig() *RuntimeConfig {
	return NewRuntimeConfigJIT()
}
