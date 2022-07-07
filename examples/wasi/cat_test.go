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
	for _, toolchain := range []string{"cargo-wasi", "tinygo", "zig-cc"} {
		toolchain := toolchain
		t.Run(toolchain, func(t *testing.T) {
			t.Setenv("TOOLCHAIN", toolchain)
			stdout, stderr := maintester.TestMain(t, main, "cat", "/test.txt")
			require.Equal(t, "", stderr)
			require.Equal(t, "greet filesystem\n", stdout)
		})
	}
}
