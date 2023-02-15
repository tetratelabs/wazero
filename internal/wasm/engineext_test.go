package wasm_test

import (
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/engineext"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// Test-time checks on the engineext interfaces implementations.
// This is necessary because these interfaces are not used in wazero source tree, but necessary
// to be defined and met to allow the out-of-source engine implementation.
var (
	_ engineext.Module           = &wasm.Module{}
	_ engineext.ModuleInstance   = &wasm.ModuleInstance{}
	_ engineext.FunctionInstance = &wasm.FunctionInstance{}
)

func TestMemoryInstanceBufferOffset(t *testing.T) {
	require.Equal(t, int(unsafe.Offsetof(wasm.MemoryInstance{}.Buffer)), engineext.MemoryInstanceBufferOffset)
}
