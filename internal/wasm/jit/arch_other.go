//go:build !amd64 && !arm64

package jit

// archContext is empty on an unsupported architecture.
type archContext struct{}
