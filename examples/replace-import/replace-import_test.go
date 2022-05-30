package main

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/maintester"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// Test_main ensures the following will work:
//
//	go run replace-import.go
func Test_main(t *testing.T) {
	stdout, _ := maintester.TestMain(t, main, "replace-import")
	require.Equal(t, "module \"needs-import\" closed with exit_code(255)\n", stdout)
}
