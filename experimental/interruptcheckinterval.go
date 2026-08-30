package experimental

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/internal/expctxkeys"
)

// WithInterruptCheckInterval configures the interrupt check interval for the compiler engine
// when WithCloseOnContextDone is enabled. Instead of checking for context cancellation on every
// loop iteration, the check is performed every N iterations, reducing overhead.
//
// The interval must be a power of 2 (e.g., 1, 2, 4, 8, 16, 32, 64). A value of 0 means
// check every iteration (default behavior). Internally, the value is used as a bitmask
// (interval - 1) for efficient modulo checking.
//
// This setting only affects the compiler engine (wazevo). The interpreter engine ignores it.
func WithInterruptCheckInterval(ctx context.Context, interval uint64) context.Context {
	if interval != 0 && (interval&(interval-1)) != 0 {
		panic(fmt.Errorf("interruptCheckInterval invalid: %d is not zero or a power of two", interval))
	}

	return context.WithValue(ctx, expctxkeys.InterruptCheckIntervalKey{}, interval)
}

// GetInterruptCheckInterval returns the interrupt check interval from context.
// Returns 0 if not set (meaning check every iteration).
func GetInterruptCheckInterval(ctx context.Context) uint64 {
	interval, _ := ctx.Value(expctxkeys.InterruptCheckIntervalKey{}).(uint64)

	return interval
}
