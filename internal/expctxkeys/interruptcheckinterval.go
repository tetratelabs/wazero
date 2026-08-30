package expctxkeys

// InterruptCheckIntervalKey is a context.Context Value key.
// Its associated value should be a uint64 representing the interrupt check interval
// for the compiler engine when WithCloseOnContextDone is enabled.
type InterruptCheckIntervalKey struct{}
