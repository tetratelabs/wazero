package main

import (
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/maintester"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// Test_main ensures the following will work:
//
// go run assemblyscript.go 7
func Test_main(t *testing.T) {
	if runtime.GOARCH == "riscv64" {
		t.Skip("Compiler not implemented on riscv64 yet.")
	}
	stdout, stderr := maintester.TestMain(t, main, "assemblyscript", "7")
	require.Equal(t, "hello_world returned: 10", stdout)
	require.Equal(t, "sad sad world at assemblyscript.ts:7:3\n", stderr)
}
