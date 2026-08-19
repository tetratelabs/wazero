//go:build !cgo || !((darwin || linux) && (amd64 || arm64))

package guardsig

// Supported returns false: without cgo (or on an unsupported platform) the
// fault handler is unavailable, and guard-page memory silently falls back to
// bounds-checked code generation.
func Supported() bool { return false }
