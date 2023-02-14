//go:build windows

package platform

import (
	"syscall"
	"unsafe"
)

var qpc = kernel32.NewProc("QueryPerformanceCounter")

// On Windows, time.Time handled in time package cannot have the nanosecond precision.
// The reason is that by default, it doesn't use QueryPerformanceCounter[1], but instead, use "interrupt time"
// which doesn't support nanoseconds precision (though it is a monotonic) [2, 3, 4, 5].
//
// [1] https://learn.microsoft.com/en-us/windows/win32/api/profileapi/nf-profileapi-queryperformancecounter
// [2] https://github.com/golang/go/blob/0cd309e12818f988693bf8e4d9f1453331dcf9f2/src/runtime/sys_windows_amd64.s#L297-L298
// [3] https://github.com/golang/go/blob/0cd309e12818f988693bf8e4d9f1453331dcf9f2/src/runtime/os_windows.go#L549-L551
// [4] https://github.com/golang/go/blob/master/src/runtime/time_windows.h#L7-L13
// [5] http://web.archive.org/web/20210411000829/https://wrkhpi.wordpress.com/2007/08/09/getting-os-information-the-kuser_shared_data-structure/
//
// Therefore, on Windows, we directly invoke the syscall for QPC instead of neither time.Now nor runtime.nanotime.
// See https://github.com/golang/go/issues/31160 for example.
func nanotime() int64 {
	var now int64
	_, _, _ = syscall.SyscallN(qpc.Addr(), 1, uintptr(unsafe.Pointer(&now)), 0, 0)
	return now
}
