// Package close is a notification hook, invoked when a module is closed.
package close

import (
	"context"

	"github.com/tetratelabs/wazero/internal/close"
)

// Notification is experimental progress towards #1197.
type Notification interface {
	// OnClose is a notification that occurs *before* an api.Module is closed.
	// `exitCode` is zero on success or in the case there was no exit code.
	//
	// Notes:
	//   - This does not return an error because the module will be closed
	//     unconditionally.
	//   - Do not panic from this function as it doing so could cause resource
	//     leaks.
	OnClose(ctx context.Context, exitCode uint32)
}

// ^-- Note: This might need to be a part of the listener, but we need to
// decide if the perf impact of doing that warrants it being a normal
// ModuleConfig/part of HostModuleBuilder.

// NotificationFunc is a convenience for defining inlining a Notification.
type NotificationFunc func(ctx context.Context, exitCode uint32)

// OnClose implements Notification.OnClose.
func (f NotificationFunc) OnClose(ctx context.Context, exitCode uint32) {
	f(ctx, exitCode)
}

// WithNotification registers the given Notification into the given
// context.Context.
func WithNotification(ctx context.Context, notification Notification) context.Context {
	if notification != nil {
		return context.WithValue(ctx, close.NotificationKey{}, notification)
	}
	return ctx
}
