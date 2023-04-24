package platform

import (
	"syscall"
	"time"
	"unsafe"
)

// wasiFdStdin is the constant value for stdin on Wasi.
// We need this constant because on Windows os.Stdin.Fd() != 0.
const wasiFdStdin = 0

// pollInterval is the interval between each calls to peekNamedPipe in pollNamedPipe
const pollInterval = 100 * time.Millisecond

// procPeekNamedPipe is the syscall.LazyProc in kernel32 for PeekNamedPipe
var procPeekNamedPipe = kernel32.NewProc("PeekNamedPipe")

// syscall_select emulates the select syscall on Windows for two, well-known cases, returns syscall.ENOSYS for all others.
// If r contains fd 0, and it is a regular file, then it immediately returns 1 (data ready on stdin)
// and r will have the fd 0 bit set.
// If r contains fd 0, and it is a FILE_TYPE_CHAR, then it invokes PeekNamedPipe to check the buffer for input;
// if there is data ready, then it returns 1 and r will have fd 0 bit set.
// If n==0 it will wait for the given timeout duration, but it will return syscall.ENOSYS if timeout is nil,
// i.e. it won't block indefinitely.
//
// Note: idea taken from https://stackoverflow.com/questions/6839508/test-if-stdin-has-input-for-c-windows-and-or-linux
// PeekNamedPipe: https://learn.microsoft.com/en-us/windows/win32/api/namedpipeapi/nf-namedpipeapi-peeknamedpipe
// "GetFileType can assist in determining what device type the handle refers to. A console handle presents as FILE_TYPE_CHAR."
// https://learn.microsoft.com/en-us/windows/console/console-handles
func syscall_select(n int, r, w, e *FdSet, timeout *time.Duration) (int, error) {
	if n == 0 {
		// don't block indefinitely
		if timeout == nil {
			return -1, syscall.ENOSYS
		}
		time.Sleep(*timeout)
		return 0, nil
	}
	if r.IsSet(wasiFdStdin) {
		fileType, err := syscall.GetFileType(syscall.Stdin)
		if err != nil {
			return 0, err
		}
		if fileType&syscall.FILE_TYPE_CHAR != 0 {
			res, err := pollNamedPipe(syscall.Stdin, timeout)
			if err != nil && err != syscall.Errno(0) {
				return -1, err
			}
			if !res {
				r.Zero()
				return 0, nil
			}
		}
		r.Zero()
		r.Set(wasiFdStdin)
		return 1, nil
	}
	return -1, syscall.ENOSYS
}

// pollNamedPipe polls the given named pipe handle for the given duration.
//
// The implementation actually polls at
func pollNamedPipe(pipeHandle syscall.Handle, duration *time.Duration) (bool, error) {
	// Short circuit when the duration is nil.
	if duration != nil && *duration == time.Duration(0) {
		return peekNamedPipe(pipeHandle)
	}
	// Ticker that emits at every pollInterval.
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()
	// If the duration is nil, then poll forever.
	if duration == nil {
		for range tick.C {
			res, err := peekNamedPipe(pipeHandle)
			if err != nil && err != syscall.Errno(0) {
				return false, err
			}
			if res {
				return res, nil
			}
		}
	} else {
		// Otherwise, leave after the given duration, and check every pollInterval.
		after := time.NewTimer(*duration)
		defer after.Stop()
		for {
			select {
			case <-after.C:
				return false, nil
			case <-tick.C:
				res, err := peekNamedPipe(pipeHandle)
				if err != nil && err != syscall.Errno(0) {
					return false, err
				}
				if res {
					return res, nil
				}
			}
		}
	}
	return false, nil
}

// [in]            HANDLE  hNamedPipe,
// [out, optional] LPVOID  lpBuffer,
// [in]            DWORD   nBufferSize,
// [out, optional] LPDWORD lpBytesRead,
// [out, optional] LPDWORD lpTotalBytesAvail,
// [out, optional] LPDWORD lpBytesLeftThisMessage
func peekNamedPipe(handle syscall.Handle) (bool, error) {
	var totalBytesAvail uint32
	_, _, err := procPeekNamedPipe.Call(
		uintptr(handle),
		0,
		0,
		0,
		uintptr(unsafe.Pointer(&totalBytesAvail)),
		0)
	return totalBytesAvail > 0, err
}
