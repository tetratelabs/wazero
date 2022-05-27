package experimental

import (
	"context"

	"github.com/tetratelabs/wazero/internal/sys"
)

// WithTimeNowUnixNano allows you to control the value otherwise returned by time.Now().UnixNano()
func WithTimeNowUnixNano(ctx context.Context, timeUnixNano func() uint64) context.Context {
	return context.WithValue(ctx, sys.TimeNowUnixNanoKey{}, timeUnixNano)
}
