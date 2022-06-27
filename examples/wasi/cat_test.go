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
	stdout, _ := maintester.TestMain(t, main, "cat", "/test.txt")
	require.Equal(t, "greet filesystem\n", stdout)
}
