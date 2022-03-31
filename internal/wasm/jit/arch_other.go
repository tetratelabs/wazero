//go:build !amd64 && !arm64

package jit

import (
	"fmt"
	"runtime"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

// archContext is empty on an unsupported architecture.
type archContext struct{}

// newCompiler returns an unsupported error.
func newCompiler(f *wasm.FunctionInstance, ir *wazeroir.CompilationResult) (compiler, error) {
	return nil, fmt.Errorf("unsupported GOARCH %s", runtime.GOARCH)
}
