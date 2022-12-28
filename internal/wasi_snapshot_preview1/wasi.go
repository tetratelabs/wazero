// Package wasi_snapshot_preview1 is an internal helper to remove package
// cycles re-using errno
package wasi_snapshot_preview1

import (
	"fmt"
)

const ModuleName = "wasi_snapshot_preview1"

// ErrnoName returns the POSIX error code name, except ErrnoSuccess, which is not an error. e.g. Errno2big -> "E2BIG"
func ErrnoName(errno uint32) string {
	if int(errno) < len(errnoToString) {
		return errnoToString[errno]
	}
	return fmt.Sprintf("errno(%d)", errno)
}

var errnoToString = [...]string{
	"ESUCCESS",
	"E2BIG",
	"EACCES",
	"EADDRINUSE",
	"EADDRNOTAVAIL",
	"EAFNOSUPPORT",
	"EAGAIN",
	"EALREADY",
	"EBADF",
	"EBADMSG",
	"EBUSY",
	"ECANCELED",
	"ECHILD",
	"ECONNABORTED",
	"ECONNREFUSED",
	"ECONNRESET",
	"EDEADLK",
	"EDESTADDRREQ",
	"EDOM",
	"EDQUOT",
	"EEXIST",
	"EFAULT",
	"EFBIG",
	"EHOSTUNREACH",
	"EIDRM",
	"EILSEQ",
	"EINPROGRESS",
	"EINTR",
	"EINVAL",
	"EIO",
	"EISCONN",
	"EISDIR",
	"ELOOP",
	"EMFILE",
	"EMLINK",
	"EMSGSIZE",
	"EMULTIHOP",
	"ENAMETOOLONG",
	"ENETDOWN",
	"ENETRESET",
	"ENETUNREACH",
	"ENFILE",
	"ENOBUFS",
	"ENODEV",
	"ENOENT",
	"ENOEXEC",
	"ENOLCK",
	"ENOLINK",
	"ENOMEM",
	"ENOMSG",
	"ENOPROTOOPT",
	"ENOSPC",
	"ENOSYS",
	"ENOTCONN",
	"ENOTDIR",
	"ENOTEMPTY",
	"ENOTRECOVERABLE",
	"ENOTSOCK",
	"ENOTSUP",
	"ENOTTY",
	"ENXIO",
	"EOVERFLOW",
	"EOWNERDEAD",
	"EPERM",
	"EPIPE",
	"EPROTO",
	"EPROTONOSUPPORT",
	"EPROTOTYPE",
	"ERANGE",
	"EROFS",
	"ESPIPE",
	"ESRCH",
	"ESTALE",
	"ETIMEDOUT",
	"ETXTBSY",
	"EXDEV",
	"ENOTCAPABLE",
}

// oflags are open flags used by path_open
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-oflags-flagsu16
const (
	// O_CREAT creates a file if it does not exist.
	O_CREAT uint16 = 1 << iota //nolint
	// O_DIRECTORY fails if not a directory.
	O_DIRECTORY
	// O_EXCL fails if file already exists.
	O_EXCL //nolint
	// O_TRUNC truncates the file to size 0.
	O_TRUNC //nolint
)

// file descriptor flags
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fdflags
const (
	FD_APPEND uint16 = 1 << iota //nolint
	FD_DSYNC
	FD_NONBLOCK
	FD_RSYNC
	FD_SYNC
)

// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#lookupflags
const (
	// LOOKUP_SYMLINK_FOLLOW expands a path if it resolves into a symbolic
	// link.
	LOOKUP_SYMLINK_FOLLOW uint16 = 1 << iota //nolint
)

// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-rights-flagsu64
const (
	// RIGHT_FD_DATASYNC is the right to invoke fd_datasync. If RIGHT_PATH_OPEN
	// is set, includes the right to invoke path_open with FD_DSYNC.
	RIGHT_FD_DATASYNC uint32 = 1 << iota //nolint

	// RIGHT_FD_READ is he right to invoke fd_read and sock_recv. If
	// RIGHT_FD_SYNC is set, includes the right to invoke fd_pread.
	RIGHT_FD_READ

	// RIGHT_FD_SEEK is the right to invoke fd_seek. This flag implies
	// RIGHT_FD_TELL.
	RIGHT_FD_SEEK

	// RIGHT_FDSTAT_SET_FLAGS is the right to invoke fd_fdstat_set_flags.
	RIGHT_FDSTAT_SET_FLAGS

	// RIGHT_FD_SYNC The right to invoke fd_sync. If path_open is set, includes
	// the right to invoke path_open with FD_RSYNC and FD_DSYNC.
	RIGHT_FD_SYNC

	// RIGHT_FD_TELL is the right to invoke fd_seek in such a way that the file
	// offset remains unaltered (i.e., whence::cur with offset zero), or to
	// invoke fd_tell.
	RIGHT_FD_TELL

	// RIGHT_FD_WRITE is the right to invoke fd_write and sock_send. If
	// RIGHT_FD_SEEK is set, includes the right to invoke fd_pwrite.
	RIGHT_FD_WRITE

	// RIGHT_FD_ADVISE is the right to invoke fd_advise.
	RIGHT_FD_ADVISE

	// RIGHT_FD_ALLOCATE is the right to invoke fd_allocate.
	RIGHT_FD_ALLOCATE

	// RIGHT_PATH_CREATE_DIRECTORY is the right to invoke
	// path_create_directory.
	RIGHT_PATH_CREATE_DIRECTORY

	// RIGHT_PATH_CREATE_FILE when RIGHT_PATH_OPEN is set, the right to invoke
	// path_open with O_CREATE.
	RIGHT_PATH_CREATE_FILE

	// RIGHT_PATH_LINK_SOURCE is the right to invoke path_link with the file
	// descriptor as the source directory.
	RIGHT_PATH_LINK_SOURCE

	// RIGHT_PATH_LINK_TARGET is the right to invoke path_link with the file
	// descriptor as the target directory.
	RIGHT_PATH_LINK_TARGET

	// RIGHT_PATH_OPEN is the right to invoke path_open.
	RIGHT_PATH_OPEN

	// RIGHT_FD_READDIR is the right to invoke fd_readdir.
	RIGHT_FD_READDIR

	// RIGHT_PATH_READLINK is the right to invoke path_readlink.
	RIGHT_PATH_READLINK

	// RIGHT_PATH_RENAME_SOURCE is the right to invoke path_rename with the
	// file descriptor as the source directory.
	RIGHT_PATH_RENAME_SOURCE

	// RIGHT_PATH_RENAME_TARGET is the right to invoke path_rename with the
	// file descriptor as the target directory.
	RIGHT_PATH_RENAME_TARGET

	// RIGHT_PATH_FILESTAT_GET is the right to invoke path_filestat_get.
	RIGHT_PATH_FILESTAT_GET

	// RIGHT_PATH_FILESTAT_SET_SIZE is the right to change a file's size (there
	// is no path_filestat_set_size). If RIGHT_PATH_OPEN is set, includes the
	// right to invoke path_open with O_TRUNC.
	RIGHT_PATH_FILESTAT_SET_SIZE

	// RIGHT_PATH_FILESTAT_SET_TIMES is the right to invoke
	// path_filestat_set_times.
	RIGHT_PATH_FILESTAT_SET_TIMES

	// RIGHT_FD_FILESTAT_GET is the right to invoke fd_filestat_get.
	RIGHT_FD_FILESTAT_GET

	// RIGHT_FD_FILESTAT_SET_SIZE is the right to invoke fd_filestat_set_size.
	RIGHT_FD_FILESTAT_SET_SIZE

	// RIGHT_FD_FILESTAT_SET_TIMES is the right to invoke
	// fd_filestat_set_times.
	RIGHT_FD_FILESTAT_SET_TIMES

	// RIGHT_PATH_SYMLINK is the right to invoke path_symlink.
	RIGHT_PATH_SYMLINK

	// RIGHT_PATH_REMOVE_DIRECTORY is the right to invoke
	// path_remove_directory.
	RIGHT_PATH_REMOVE_DIRECTORY

	// RIGHT_PATH_UNLINK_FILE is the right to invoke path_unlink_file.
	RIGHT_PATH_UNLINK_FILE

	// RIGHT_POLL_FD_READWRITE when RIGHT_FD_READ is set, includes the right to
	// invoke poll_oneoff to subscribe to eventtype::fd_read. If RIGHT_FD_WRITE
	// is set, includes the right to invoke poll_oneoff to subscribe to
	// eventtype::fd_write.
	RIGHT_POLL_FD_READWRITE

	// RIGHT_SOCK_SHUTDOWN is the right to invoke sock_shutdown.
	RIGHT_SOCK_SHUTDOWN
)

const (
	FILETYPE_UNKNOWN uint8 = iota
	FILETYPE_BLOCK_DEVICE
	FILETYPE_CHARACTER_DEVICE
	FILETYPE_DIRECTORY
	FILETYPE_REGULAR_FILE
	FILETYPE_SOCKET_DGRAM
	FILETYPE_SOCKET_STREAM
	FILETYPE_SYMBOLIC_LINK
)

// FiletypeName returns string name of the file type.
func FiletypeName(filetype uint8) string {
	if int(filetype) < len(filetypeToString) {
		return filetypeToString[filetype]
	}
	return fmt.Sprintf("filetype(%d)", filetype)
}

var filetypeToString = [...]string{
	"UNKNOWN",
	"BLOCK_DEVICE",
	"CHARACTER_DEVICE",
	"DIRECTORY",
	"REGULAR_FILE",
	"SOCKET_DGRAM",
	"SOCKET_STREAM",
	"SYMBOLIC_LINK",
}
