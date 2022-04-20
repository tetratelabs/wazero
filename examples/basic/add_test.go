package add

import (
	"testing"

	"github.com/heeus/hwazero/internal/testing/maintester"
	"github.com/heeus/hwazero/internal/testing/require"
)

// Test_main ensures the following will work:
//
//	go run add.go 7 9
func Test_main(t *testing.T) {
	stdout, _ := maintester.TestMain(t, main, "add", "7", "9")
	require.Equal(t, `wasm/math: 7 + 9 = 16
host/math: 7 + 9 = 16
`, stdout)
}
