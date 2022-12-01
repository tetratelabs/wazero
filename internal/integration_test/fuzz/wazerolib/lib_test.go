package main

import (
	"os"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

// TestReRunFailedCase re-runs the failed case specified by WASM_BINARY_NAME in testdata directory.
func TestReRunFailedCase(t *testing.T) {
	binaryPath := os.Getenv("WASM_BINARY_PATH")

	wasmBin, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatal(err)
	}

	requireNoDiff(wasmBin, func(err error) { require.NoError(t, err) })
}
