package fsapi

import "syscall"

// See https://github.com/illumos/illumos-gate/blob/edd580643f2cf1434e252cd7779e83182ea84945/usr/src/uts/common/sys/fcntl.h#L90
const (
	O_DIRECTORY = 1 << 29
	O_NOFOLLOW  = 1 << 30
	O_NONBLOCK  = syscall.O_NONBLOCK
)
