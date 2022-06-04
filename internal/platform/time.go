package platform

import (
	"context"
	"time"
)

const FakeEpochNanos = int64(1640995200000000000) // midnight UTC 2022-01-01

// FakeWalltime implements sys.Walltime with FakeEpochNanos.
func FakeWalltime(context.Context) (sec int64, nsec int32) {
	return FakeEpochNanos / 1e9, int32(FakeEpochNanos % 1e9)
}

// FakeNanotime implements sys.Nanotime with FakeEpochNanos.
func FakeNanotime(context.Context) int64 {
	return FakeEpochNanos
}

// Walltime implements sys.Walltime with time.Now.
//
// Note: This is only notably less efficient than it could be is reading
// runtime.walltime(). time.Now defensively reads nanotime also, just in case
// time.Since is used. This doubles the performance impact. However, wall time
// is likely to be read less frequently than Nanotime. Also, doubling the cost
// matters less on fast platforms that can return both in <=100ns.
func Walltime(context.Context) (sec int64, nsec int32) {
	t := time.Now()
	return t.Unix(), int32(t.Nanosecond())
}

// nanoBase uses time.Now to ensure a monotonic clock reading on all platforms
// via time.Since.
var nanoBase = time.Now()

// nanotimePortable implements sys.Nanotime with time.Since.
//
// Note: This is less efficient than it could be is reading runtime.nanotime(),
// Just to do that requires CGO.
func nanotimePortable() int64 {
	return time.Since(nanoBase).Nanoseconds()
}

// Nanotime implements sys.Nanotime with runtime.nanotime() if CGO is available
// and time.Since if not.
func Nanotime(context.Context) int64 {
	return nanotime()
}
