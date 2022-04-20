package main

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/maintester"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// Test_main ensures the following will work:
//
//	go run say-hello.go wazero
func Test_main(t *testing.T) {
	stdout, _ := maintester.TestMain(t, main, "say-hello", "wazero")
	require.Equal(t, `Hello, wazero!
`, stdout)
}
