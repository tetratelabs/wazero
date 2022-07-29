package main

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/maintester"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// Test_main ensures the following will work:
//
//	go run multiple-results.go
func Test_main(t *testing.T) {
	stdout, _ := maintester.TestMain(t, main, "multiple-results")
	require.Equal(t, `result-offset/wasm: age=37
multi-value/wasm: age=37
multi-value/imported_host: age=37
`, stdout)
}
