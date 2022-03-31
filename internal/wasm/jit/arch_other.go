//go:build !amd64 && !arm64

package jit

import (
	"fmt"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

// archContext is empty on an unsupported architecture.
type archContext struct{}

// newCompiler returns a new compiler interface which can be used to compile the given function instance.
// Note: ir param can be nil for host functions.
func newCompiler(f *wasm.FunctionInstance, ir *wazeroir.CompilationResult) (compiler, error) {
	return nil, fmt.Errorf("unsupported GOARCH %s", arch)
}
