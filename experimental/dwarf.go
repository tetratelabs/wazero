package experimental

import (
	"context"
)

type enableDWARFBasedStackTraceKey struct{}

func WithDWARFBasedStackTrace(ctx context.Context) context.Context {
	return context.WithValue(ctx, enableDWARFBasedStackTraceKey{}, struct{}{})
}

func DWARFBasedStackTraceEnabled(ctx context.Context) bool {
	return ctx.Value(enableDWARFBasedStackTraceKey{}) != nil
}
