package experimental_test

import (
	"context"

	"github.com/tetratelabs/wazero/experimental"
)

var ctx context.Context

// This shows how to implement a custom cleanup task on close.
func Example_closeNotifier() {
	closeCh := make(chan struct{})
	ctx = experimental.WithCloseNotifier(
		context.Background(),
		experimental.CloseNotifyFunc(func(context.Context, uint32) { close(closeCh) }),
	)

	// ... create module, do some work. Sometime later in another goroutine:

	select {
	case <-closeCh:
		// do some cleanup
	default:
		// do some more work with the module
	}

	// Output:
}
