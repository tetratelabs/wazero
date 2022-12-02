package main

import (
	"os"
	"testing"
)

// TestReRunFailedValidateCase re-runs the failed case specified by WASM_BINARY_NAME in testdata directory.
func TestReRunFailedValidateCase(t *testing.T) {
	binaryPath := os.Getenv("WASM_BINARY_PATH")

	wasmBin, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Skip(err)
	}

	tryCompile(wasmBin)
}
