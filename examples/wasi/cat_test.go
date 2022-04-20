package wasi_example

import (
	"testing"

	"github.com/heeus/hwazero/internal/testing/maintester"
	"github.com/heeus/hwazero/internal/testing/require"
)

// Test_main ensures the following will work:
//
//	go run cat.go ./test.txt
func Test_main(t *testing.T) {
	stdout, _ := maintester.TestMain(t, main, "cat", "./test.txt")
	require.Equal(t, "hello filesystem\n", stdout)
}
