package main

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/maintester"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// Test_main ensures the following will work:
//
//	go run age-calculator.go 2000
func Test_main(t *testing.T) {
	// Set ENV to ensure this test doesn't need maintenance every year.
	t.Setenv("CURRENT_YEAR", "2021")

	stdout, _ := maintester.TestMain(t, main, "age-calculator", "2000")
	require.Equal(t, `println >> 21
log_i32 >> 21
`, stdout)
}
