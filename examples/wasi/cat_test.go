package main

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/maintester"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// Test_main ensures the following will work:
//
//	go run cat.go /test.txt
func Test_main(t *testing.T) {
	for _, compiler := range []string{"tinygo", "zig-cc"} {
		compiler := compiler
		t.Run(compiler, func(t *testing.T) {
			t.Setenv("WASM_COMPILER", compiler)
			stdout, stderr := maintester.TestMain(t, main, "cat", "/test.txt")
			require.Equal(t, "", stderr)
			require.Equal(t, "greet filesystem\n", stdout)
		})
	}
}
