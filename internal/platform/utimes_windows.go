package platform

import "syscall"

func futimens(fd uintptr, atimeNsec, mtimeNsec int64) error {
	// Attempt to get the stat by handle, which works for normal files
	h := syscall.Handle(fd)

	// Perform logic similar to what's done in syscall.UtimesNano
	a := syscall.NsecToFiletime(atimeNsec)
	w := syscall.NsecToFiletime(mtimeNsec)
	return syscall.SetFileTime(h, nil, &a, &w)
}
