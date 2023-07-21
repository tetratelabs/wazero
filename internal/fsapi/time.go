package fsapi

// The following constants make it possible to portably handle utimes, at the
// cost of making it impossible to set a time one or two nanoseconds before
// epoch.

const (
	// UTIME_NOW is a special syscall.Timespec NSec value used to set the
	// file's timestamp to a value close to, but not greater than the current
	// system time.
	UTIME_NOW = -1

	// UTIME_OMIT is a special syscall.Timespec NSec value used to avoid
	// setting the file's timestamp.
	UTIME_OMIT = -2
)
