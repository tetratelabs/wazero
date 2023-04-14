package wasi_snapshot_preview1

import (
	"context"
	"errors"
	"syscall"
	"time"
)

type FDSet syscall.FdSet

var nullFdSet = &FDSet{}

func (fds *FDSet) Zero() {
	copy(fds.Bits[:], (nullFdSet).Bits[:])
}

func (fds *FDSet) Set(fd uintptr) {
	fds.Bits[fd/NFDBITS] |= (1 << (fd % NFDBITS))
}

func (fds *FDSet) IsSet(fd uintptr) bool {
	return fds.Bits[fd/NFDBITS]&(1<<(fd%NFDBITS)) != 0
}

func selectStdinTimeout(ctx context.Context, stdin bool, timeout time.Duration) syscall.Errno {
	var fdSet *FDSet
	n := 0
	if stdin {
		fdSet = &FDSet{}
		fdSet.Set(0)
		n = 1
	}

	var timeval = syscall.Timeval{
		Sec:       int64(timeout.Seconds()),
		Usec:      1,
		Pad_cgo_0: [4]byte{},
	}

	fds := (*syscall.FdSet)(fdSet)
	err := syscall.Select(n, fds, nil, nil, &timeval)

	if fdSet != nil && fdSet.IsSet(0) {
		return 0
	} else {
		return syscall.EAGAIN
	}

	switch {
	case err == nil:
		return 0
	case errors.Is(err, syscall.EAGAIN):
		return syscall.EAGAIN
	case errors.Is(err, syscall.EINVAL):
		return syscall.EINVAL
	case errors.Is(err, syscall.ENOENT):
		return syscall.ENOENT
	}
	panic(err)

}
