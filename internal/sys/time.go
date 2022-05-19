package sys

// TimeNowUnixNanoKey is a context.Context Value key. Its associated value should be a func() uint64.
//
// See https://github.com/tetratelabs/wazero/issues/491
type TimeNowUnixNanoKey struct{}
