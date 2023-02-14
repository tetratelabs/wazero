//go:build !cgo && !windows

package platform

func nanotime() int64 {
	return nanotimePortable()
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
