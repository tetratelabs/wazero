package main

import (
	"os"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

// TestReRunFailedRequireNoDiffCase re-runs the failed case specified by WASM_BINARY_NAME in testdata directory.
func TestReRunFailedRequireNoDiffCase(t *testing.T) {
	// binaryPath := os.Getenv("WASM_BINARY_PATH")

	wasmBin, err := os.ReadFile("/Users/mathetake/wazero/internal/integration_test/fuzz/wazerolib/testdata/750512a04967c7a63b2b7f16cca7d23dcea67341602f884bc8bff73abc68baf7.wasm")
	if err != nil {
		t.Skip(err)
	}

	requireNoDiff(wasmBin, func(err error) { require.NoError(t, err) })
}
