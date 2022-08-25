package main

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/maintester"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// Test_main ensures the following will work:
//
//	go run stars.go
func Test_main(t *testing.T) {
	stdout, stderr := maintester.TestMain(t, main, "stars")
	require.Equal(t, "", stderr)
	require.Equal(t, "wazero has 9999999 stars. Does that include you?\n", stdout)
}
