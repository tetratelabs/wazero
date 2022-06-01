// Note: The build constraints here are about the compiler, which is more
// narrow than the architectures supported by the assembler.
//
// For example, the internal/platform package has dependencies such as
// syscall.Mprotect when not on Windows. Go's SDK only supports this on darwin
// and linux. Constraints here may loosen as the platform package supports
// other runtime.GOOS, or a plugin becomes available to accomplish the same.
//
// Meanwhile, users who know their runtime.GOOS can operate with the compiler
// may choose to use NewRuntimeConfigCompiler explicitly.
//go:build (amd64 || arm64) && (darwin || linux || windows)

package wazero

func newRuntimeConfig() RuntimeConfig {
	return NewRuntimeConfigCompiler()
}
