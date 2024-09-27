//go:build !tinygo

package sysfs

// If pathname is a symbolic link, do not dereference it:
// instead return information about the link itself, like lstat(2).
const AT_SYMLINK_NOFOLLOW = 0x100
