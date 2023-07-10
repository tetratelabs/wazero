package close

import "context"

// NotificationKey is a context.Context Value key. Its associated value should
// be a Notification.
type NotificationKey struct{}

type Notification interface {
	OnClose(ctx context.Context, exitCode uint32)
}
