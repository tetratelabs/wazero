// Package close allows experimental.CloseNotifier without introducing a
// package cycle.
package ctxkey

import "context"

// CloseNotifierKey is a context.Context Value key. Its associated value should be a
// Notifier.
type CloseNotifierKey struct{}

type Notifier interface {
	CloseNotify(ctx context.Context, exitCode uint32)
}
