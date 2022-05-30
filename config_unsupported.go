//go:build !amd64 && !arm64

package wazero

func newRuntimeConfig() RuntimeConfig {
	return NewRuntimeConfigInterpreter()
}
