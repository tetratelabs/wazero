package main

import (
	"os"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/nodiff"
)

// TestReRunFailedRequireNoDiffCase re-runs the failed case specified by WASM_BINARY_NAME in testdata directory.
func TestReRunFailedRequireNoDiffCase(t *testing.T) {
	binaryPath := os.Getenv("WASM_BINARY_PATH")

	wasmBin, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Skip(err)
	}

	nodiff.RequireNoDiffT(t, wasmBin, true, true)
}
