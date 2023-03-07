package platform

import "syscall"

func futimens(fd uintptr, atimeNsec, mtimeNsec int64) error {
	// Before Go 1.20, ERROR_INVALID_HANDLE was returned for too many reasons.
	// Kick out so that callers can use path-based operations instead.
	if !IsGo120 {
		return syscall.ENOSYS
	}

	// Attempt to get the stat by handle, which works for normal files
	h := syscall.Handle(fd)

	// Perform logic similar to what's done in syscall.UtimesNano
	a := syscall.NsecToFiletime(atimeNsec)
	w := syscall.NsecToFiletime(mtimeNsec)

	// Note: This returns ERROR_ACCESS_DENIED when the input is a directory.
	return syscall.SetFileTime(h, nil, &a, &w)
}
