package jit

import (
	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/jit/asm"
	"github.com/tetratelabs/wazero/internal/wasm/jit/asm/amd64"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

// init initializes variables for the amd64 architecture
func init() {
	newArchContext = newArchContextImpl
}

// archContext is embedded in callEngine in order to store architecture-specific data.
// For amd64, this is empty.
type archContext struct{}

// newArchContextImpl implements newArchContext for amd64 architecture.
func newArchContextImpl() (ret archContext) { return }
