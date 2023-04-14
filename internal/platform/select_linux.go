package platform

import (
	"syscall"
	"time"
)

func SelectTimeout(timeout time.Duration) error {
	_, err := Select(0, nil, nil, nil, timeout)
	return err
}

func SelectStdin(timeout time.Duration) (bool, error) {
	fdSet := &FDSet{}
	fdSet.Set(0)
	count, err := Select(1, fdSet, nil, nil, timeout)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func Select(n int, r, w, e *FDSet, timeout time.Duration) (int, error) {
	t := syscall.NsecToTimeval(timeout.Nanoseconds())
	return syscall.Select(n, (*syscall.FdSet)(r), (*syscall.FdSet)(w), (*syscall.FdSet)(e), &t)
}

// Code lifted from https://github.com/creack/goselect (MIT licensed)
// Ports the macros FD_SET, FD_CLEAR, etc. to methods

// FDSet wraps syscall.FdSet with convenience methods
type FDSet syscall.FdSet

// Set adds the fd to the set
func (fds *FDSet) Set(fd uintptr) {
	fds.Bits[fd/NFDBITS] |= (1 << (fd % NFDBITS))
}

// Clear remove the fd from the set
func (fds *FDSet) Clear(fd uintptr) {
	fds.Bits[fd/NFDBITS] &^= (1 << (fd % NFDBITS))
}

// IsSet check if the given fd is set
func (fds *FDSet) IsSet(fd uintptr) bool {
	return fds.Bits[fd/NFDBITS]&(1<<(fd%NFDBITS)) != 0
}

// Keep a null set to avoid reinstatiation
var nullFdSet = &FDSet{}

// Zero empties the Set
func (fds *FDSet) Zero() {
	copy(fds.Bits[:], (nullFdSet).Bits[:])
}
