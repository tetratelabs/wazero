package ctxkey

// EnableSnapshotterKey is a context key to indicate that snapshotting should be enabled.
// The context.Context passed to a exported function invocation should have this key set
// to a non-nil value, and host functions will be able to retrieve it using SnapshotterKey.
type EnableSnapshotterKey struct{}
