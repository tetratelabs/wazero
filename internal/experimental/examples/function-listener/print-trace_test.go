package print_trace

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/maintester"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// Test_main ensures the following will work:
//
//	go run print-trace.go
func Test_main(t *testing.T) {
	stdout, _ := maintester.TestMain(t, main)
	require.Equal(t, `listener.wasm1(i32)
env.host1(i32)
host_only(i32)
listener.wasm2(i32)
env.print_trace()
`, stdout)
}
