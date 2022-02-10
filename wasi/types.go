package wasi

import "fmt"

// Errno are the error codes returned by WASI functions.
//
// Note: Codes are defined even when not relevant to WASI for use in higher-level libraries or alignment with POSIX.
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-errno-enumu16
// See https://linux.die.net/man/3/errno
type Errno uint32

// Error returns the POSIX error code name for the given Errno. Ex ESUCCESS = "ESUCCESS"
func (err Errno) Error() string {
	if int(err) < len(errnoToString) {
		return errnoToString[err]
	}
	return fmt.Sprintf("errno(%d)", uint32(err))
}

// Note: Below prefers POSIX symbol names over WASI ones, even if the docs are from WASI.
// See https://linux.die.net/man/3/errno
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#variants-1
const (
	// ESUCCESS No error occurred. System call completed successfully.
	ESUCCESS Errno = iota
	// E2BIG Argument list too long.
	E2BIG
	// EACCES Permission denied.
	EACCES
	// EADDRINUSE Address in use.
	EADDRINUSE
	// EADDRNOTAVAIL Address not available.
	EADDRNOTAVAIL
	// EAFNOSUPPORT Address family not supported.
	EAFNOSUPPORT
	// EAGAIN Resource unavailable, or operation would block.
	EAGAIN
	// EALREADY Connection already in progress.
	EALREADY
	// EBADF Bad file descriptor.
	EBADF
	// EBADMSG Bad message.
	EBADMSG
	// EBUSY Device or resource busy.
	EBUSY
	// ECANCELED Operation canceled.
	ECANCELED
	// ECHILD No child processes.
	ECHILD
	// ECONNABORTED Connection aborted.
	ECONNABORTED
	// ECONNREFUSED Connection refused.
	ECONNREFUSED
	// ECONNRESET Connection reset.
	ECONNRESET
	// EDEADLK Resource deadlock would occur.
	EDEADLK
	// EDESTADDRREQ Destination address required.
	EDESTADDRREQ
	// EDOM Mathematics argument out of domain of function.
	EDOM
	// EDQUOT Reserved.
	EDQUOT
	// EEXIST File exists.
	EEXIST
	// EFAULT Bad address.
	EFAULT
	// EFBIG File too large.
	EFBIG
	// EHOSTUNREACH Host is unreachable.
	EHOSTUNREACH
	// EIDRM Identifier removed.
	EIDRM
	// EILSEQ Illegal byte sequence.
	EILSEQ
	// EINPROGRESS Operation in progress.
	EINPROGRESS
	// EINTR Interrupted function.
	EINTR
	// EINVAL Invalid argument.
	EINVAL
	// EIO I/O error.
	EIO
	// EISCONN Socket is connected.
	EISCONN
	// EISDIR Is a directory.
	EISDIR
	// ELOOP Too many levels of symbolic links.
	ELOOP
	// EMFILE File descriptor value too large.
	EMFILE
	// EMLINK Too many links.
	EMLINK
	// EMSGSIZE Message too large.
	EMSGSIZE
	// EMULTIHOP Reserved.
	EMULTIHOP
	// ENAMETOOLONG Filename too long.
	ENAMETOOLONG
	// ENETDOWN Network is down.
	ENETDOWN
	// ENETRESET Connection aborted by network.
	ENETRESET
	// ENETUNREACH Network unreachable.
	ENETUNREACH
	// ENFILE Too many files open in system.
	ENFILE
	// ENOBUFS No buffer space available.
	ENOBUFS
	// ENODEV No such device.
	ENODEV
	// ENOENT No such file or directory.
	ENOENT
	// ENOEXEC Executable file format error.
	ENOEXEC
	// ENOLCK No locks available.
	ENOLCK
	// ENOLINK Reserved.
	ENOLINK
	// ENOMEM Not enough space.
	ENOMEM
	// ENOMSG No message of the desired type.
	ENOMSG
	// ENOPROTOOPT No message of the desired type.
	ENOPROTOOPT
	// ENOSPC No space left on device.
	ENOSPC
	// ENOSYS Function not supported.
	ENOSYS
	// ENOTCONN The socket is not connected.
	ENOTCONN
	// ENOTDIR Not a directory or a symbolic link to a directory.
	ENOTDIR
	// ENOTEMPTY Directory not empty.
	ENOTEMPTY
	// ENOTRECOVERABLE State not recoverable.
	ENOTRECOVERABLE
	// ENOTSOCK Not a socket.
	ENOTSOCK
	// ENOTSUP Not supported, or operation not supported on socket.
	ENOTSUP
	// ENOTTY Inappropriate I/O control operation.
	ENOTTY
	// ENXIO No such device or address.
	ENXIO
	// EOVERFLOW Value too large to be stored in data type.
	EOVERFLOW
	// EOWNERDEAD Previous owner died.
	EOWNERDEAD
	// EPERM Operation not permitted.
	EPERM
	// EPIPE Broken pipe.
	EPIPE
	// EPROTO Protocol error.
	EPROTO
	// EPROTONOSUPPORT Protocol error.
	EPROTONOSUPPORT
	// EPROTOTYPE Protocol wrong type for socket.
	EPROTOTYPE
	// ERANGE Result too large.
	ERANGE
	// EROFS Read-only file system.
	EROFS
	// ESPIPE Invalid seek.
	ESPIPE
	// ESRCH No such process.
	ESRCH
	// ESTALE Reserved.
	ESTALE
	// ETIMEDOUT Connection timed out.
	ETIMEDOUT
	// ETXTBSY Text file busy.
	ETXTBSY
	// EXDEV Cross-device link.
	EXDEV
	// ENOTCAPABLE Extension: Capabilities insufficient.
	ENOTCAPABLE
)

var errnoToString = [...]string{
	ESUCCESS:        "ESUCCESS",
	E2BIG:           "E2BIG",
	EACCES:          "EACCES",
	EADDRINUSE:      "EADDRINUSE",
	EADDRNOTAVAIL:   "EADDRNOTAVAIL",
	EAFNOSUPPORT:    "EAFNOSUPPORT",
	EAGAIN:          "EAGAIN",
	EALREADY:        "EALREADY",
	EBADF:           "EBADF",
	EBADMSG:         "EBADMSG",
	EBUSY:           "EBUSY",
	ECANCELED:       "ECANCELED",
	ECHILD:          "ECHILD",
	ECONNABORTED:    "ECONNABORTED",
	ECONNREFUSED:    "ECONNREFUSED",
	ECONNRESET:      "ECONNRESET",
	EDEADLK:         "EDEADLK",
	EDESTADDRREQ:    "EDESTADDRREQ",
	EDOM:            "EDOM",
	EDQUOT:          "EDQUOT",
	EEXIST:          "EEXIST",
	EFAULT:          "EFAULT",
	EFBIG:           "EFBIG",
	EHOSTUNREACH:    "EHOSTUNREACH",
	EIDRM:           "EIDRM",
	EILSEQ:          "EILSEQ",
	EINPROGRESS:     "EINPROGRESS",
	EINTR:           "EINTR",
	EINVAL:          "EINVAL",
	EIO:             "EIO",
	EISCONN:         "EISCONN",
	EISDIR:          "EISDIR",
	ELOOP:           "ELOOP",
	EMFILE:          "EMFILE",
	EMLINK:          "EMLINK",
	EMSGSIZE:        "EMSGSIZE",
	EMULTIHOP:       "EMULTIHOP",
	ENAMETOOLONG:    "ENAMETOOLONG",
	ENETDOWN:        "ENETDOWN",
	ENETRESET:       "ENETRESET",
	ENETUNREACH:     "ENETUNREACH",
	ENFILE:          "ENFILE",
	ENOBUFS:         "ENOBUFS",
	ENODEV:          "ENODEV",
	ENOENT:          "ENOENT",
	ENOEXEC:         "ENOEXEC",
	ENOLCK:          "ENOLCK",
	ENOLINK:         "ENOLINK",
	ENOMEM:          "ENOMEM",
	ENOMSG:          "ENOMSG",
	ENOPROTOOPT:     "ENOPROTOOPT",
	ENOSPC:          "ENOSPC",
	ENOSYS:          "ENOSYS",
	ENOTCONN:        "ENOTCONN",
	ENOTDIR:         "ENOTDIR",
	ENOTEMPTY:       "ENOTEMPTY",
	ENOTRECOVERABLE: "ENOTRECOVERABLE",
	ENOTSOCK:        "ENOTSOCK",
	ENOTSUP:         "ENOTSUP",
	ENOTTY:          "ENOTTY",
	ENXIO:           "ENXIO",
	EOVERFLOW:       "EOVERFLOW",
	EOWNERDEAD:      "EOWNERDEAD",
	EPERM:           "EPERM",
	EPIPE:           "EPIPE",
	EPROTO:          "EPROTO",
	EPROTONOSUPPORT: "EPROTONOSUPPORT",
	EPROTOTYPE:      "EPROTOTYPE",
	ERANGE:          "ERANGE",
	EROFS:           "EROFS",
	ESPIPE:          "ESPIPE",
	ESRCH:           "ESRCH",
	ESTALE:          "ESTALE",
	ETIMEDOUT:       "ETIMEDOUT",
	ETXTBSY:         "ETXTBSY",
	EXDEV:           "EXDEV",
	ENOTCAPABLE:     "ENOTCAPABLE",
}

const (
	// FunctionArgsGet reads command-line argument data.
	//
	// In WebAssembly 1.0 (MVP) Text format, this signature is:
	//	(import "wasi_snapshot_preview1" "args_get"
	//		(func $wasi.args_get (param $argv i32) (param $argv_buf i32) (result (;errno;) i32)))
	//
	// See API.ArgsGet
	// See FunctionArgsSizesGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-args_getargv-pointerpointeru8-argv_buf-pointeru8---errno
	FunctionArgsGet = "args_get"

	// FunctionArgsSizesGet returns command-line argument data sizes.
	//
	// In WebAssembly 1.0 (MVP) Text format, this signature is:
	//	(import "wasi_snapshot_preview1" "args_sizes_get"
	//		(func $wasi.args_sizes_get (param $result.argc i32) (param $result.argv_buf_size i32) (result (;errno;) i32)))
	//
	// See API.ArgsSizesGet
	// See FunctionArgsGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-args_sizes_get---errno-size-size
	FunctionArgsSizesGet = "args_sizes_get"

	// FunctionEnvironGet reads environment variable data.
	//
	// In WebAssembly 1.0 (MVP) Text format, this signature is:
	//	(import "wasi_snapshot_preview1" "environ_get"
	//		(func $wasi.environ_get (param $environ i32) (param $environ_buf i32) (result (;errno;) i32)))
	//
	// See API.EnvironGet
	// See FunctionEnvironSizesGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-environ_getenviron-pointerpointeru8-environ_buf-pointeru8---errno
	FunctionEnvironGet = "environ_get"

	// FunctionEnvironSizesGet returns environment variable data sizes.
	//
	// In WebAssembly 1.0 (MVP) Text format, this signature is:
	//	(import "wasi_snapshot_preview1" "environ_sizes_get"
	//		(func $wasi.environ_sizes_get (param $result.environc i32) (param $result.environBufSize i32) (result (;errno;) i32)))
	//
	// See API.EnvironSizesGet
	// See FunctionEnvironGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-environ_sizes_get---errno-size-size
	FunctionEnvironSizesGet = "environ_sizes_get"

	// FunctionClockResGet returns the resolution of a clock.
	//
	// In WebAssembly 1.0 (MVP) Text format, this signature is:
	//	(import "wasi_snapshot_preview1" "clock_res_get"
	//		(func $wasi.clock_res_get (param $id i32) (param $result.resolution i32) (result (;errno;) i32)))
	//
	// See API.ClockResGet
	// See FunctionClockTimeGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_res_getid-clockid---errno-timestamp
	FunctionClockResGet = "clock_res_get"

	// FunctionClockTimeGet returns the time value of a clock.
	//
	// In WebAssembly 1.0 (MVP) Text format, this signature is:
	//	(import "wasi_snapshot_preview1" "clock_time_get"
	//		(func $wasi.clock_time_get (param $id i32) (param $precision i64) (param $result.timestamp i32) (result (;errno;) i32)))
	//
	// See API.ClockResGet
	// See FunctionClockResGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_time_getid-clockid-precision-timestamp---errno-timestamp
	FunctionClockTimeGet = "clock_time_get"

	FunctionFDAdvise             = "fd_advise"
	FunctionFDAllocate           = "fd_allocate"
	FunctionFDClose              = "fd_close"
	FunctionFDDataSync           = "fd_datasync"
	FunctionFDFDStatGet          = "fd_fdstat_get"
	FunctionFDFDStatSetFlags     = "fd_fdstat_set_flags"
	FunctionFDFDStatSetRights    = "fd_fdstat_set_rights"
	FunctionFDFilestatGet        = "fd_filestat_get"
	FunctionFDFilestatSetSize    = "fd_filestat_set_size"
	FunctionFDFilestatSetTimes   = "fd_filestat_set_times"
	FunctionFDPread              = "fd_pread"
	FunctionFDPrestatGet         = "fd_prestat_get"
	FunctionFDPrestatDirName     = "fd_prestat_dir_name"
	FunctionFDPwrite             = "fd_pwrite"
	FunctionFDRead               = "fd_read"
	FunctionFDReaddir            = "fd_readdir"
	FunctionFDRenumber           = "fd_renumber"
	FunctionFDSeek               = "fd_seek"
	FunctionFDSync               = "fd_sync"
	FunctionFDTell               = "fd_tell"
	FunctionFDWrite              = "fd_write"
	FunctionPathCreateDirectory  = "path_create_directory"
	FunctionPathFilestatGet      = "path_filestat_get"
	FunctionPathFilestatSetTimes = "path_filestat_set_times"
	FunctionPathLink             = "path_link"
	FunctionPathOpen             = "path_open"
	FunctionPathReadlink         = "path_readlink"
	FunctionPathRemoveDirectory  = "path_remove_directory"
	FunctionPathRename           = "path_rename"
	FunctionPathSymlink          = "path_symlink"
	FunctionPathUnlinkFile       = "path_unlink_file"
	FunctionPollOneoff           = "poll_oneoff"
	FunctionProcExit             = "proc_exit"
	FunctionProcRaise            = "proc_raise"
	FunctionSchedYield           = "sched_yield"
	FunctionRandomGet            = "random_get"
	FunctionSockRecv             = "sock_recv"
	FunctionSockSend             = "sock_send"
	FunctionSockShutdown         = "sock_shutdown"
)
