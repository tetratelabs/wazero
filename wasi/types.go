package wasi

import "fmt"

// Errno are the error codes returned by WASI functions.
//
// Note: Codes are defined even when not relevant to WASI for use in higher-level libraries or alignment with POSIX.
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-errno-enumu16
// See https://linux.die.net/man/3/errno
type Errno uint32

// Error returns the POSIX error code name, except ErrnoSuccess, which isn't defined. Ex. Errno2big -> "E2BIG"
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
	// ErrnoSuccess No error occurred. System call completed successfully.
	ErrnoSuccess Errno = iota
	// Errno2big Argument list too long.
	Errno2big
	// ErrnoAcces Permission denied.
	ErrnoAcces
	// ErrnoAddrinuse Address in use.
	ErrnoAddrinuse
	// ErrnoAddrnotavail Address not available.
	ErrnoAddrnotavail
	// ErrnoAfnosupport Address family not supported.
	ErrnoAfnosupport
	// ErrnoAgain Resource unavailable, or operation would block.
	ErrnoAgain
	// ErrnoAlready Connection already in progress.
	ErrnoAlready
	// ErrnoBadf Bad file descriptor.
	ErrnoBadf
	// ErrnoBadmsg Bad message.
	ErrnoBadmsg
	// ErrnoBusy Device or resource busy.
	ErrnoBusy
	// ErrnoCanceled Operation canceled.
	ErrnoCanceled
	// ErrnoChild No child processes.
	ErrnoChild
	// ErrnoConnaborted Connection aborted.
	ErrnoConnaborted
	// ErrnoConnrefused Connection refused.
	ErrnoConnrefused
	// ErrnoConnreset Connection reset.
	ErrnoConnreset
	// ErrnoDeadlk Resource deadlock would occur.
	ErrnoDeadlk
	// ErrnoDestaddrreq Destination address required.
	ErrnoDestaddrreq
	// ErrnoDom Mathematics argument out of domain of function.
	ErrnoDom
	// ErrnoDquot Reserved.
	ErrnoDquot
	// ErrnoExist File exists.
	ErrnoExist
	// ErrnoFault Bad address.
	ErrnoFault
	// ErrnoFbig File too large.
	ErrnoFbig
	// ErrnoHostunreach Host is unreachable.
	ErrnoHostunreach
	// ErrnoIdrm Identifier removed.
	ErrnoIdrm
	// ErrnoIlseq Illegal byte sequence.
	ErrnoIlseq
	// ErrnoInprogress Operation in progress.
	ErrnoInprogress
	// ErrnoIntr Interrupted function.
	ErrnoIntr
	// ErrnoInval Invalid argument.
	ErrnoInval
	// ErrnoIo I/O error.
	ErrnoIo
	// ErrnoIsconn Socket is connected.
	ErrnoIsconn
	// ErrnoIsdir Is a directory.
	ErrnoIsdir
	// ErrnoLoop Too many levels of symbolic links.
	ErrnoLoop
	// ErrnoMfile File descriptor value too large.
	ErrnoMfile
	// ErrnoMlink Too many links.
	ErrnoMlink
	// ErrnoMsgsize Message too large.
	ErrnoMsgsize
	// ErrnoMultihop Reserved.
	ErrnoMultihop
	// ErrnoNametoolong Filename too long.
	ErrnoNametoolong
	// ErrnoNetdown Network is down.
	ErrnoNetdown
	// ErrnoNetreset Connection aborted by network.
	ErrnoNetreset
	// ErrnoNetunreach Network unreachable.
	ErrnoNetunreach
	// ErrnoNfile Too many files open in system.
	ErrnoNfile
	// ErrnoNobufs No buffer space available.
	ErrnoNobufs
	// ErrnoNodev No such device.
	ErrnoNodev
	// ErrnoNoent No such file or directory.
	ErrnoNoent
	// ErrnoNoexec Executable file format error.
	ErrnoNoexec
	// ErrnoNolck No locks available.
	ErrnoNolck
	// ErrnoNolink Reserved.
	ErrnoNolink
	// ErrnoNomem Not enough space.
	ErrnoNomem
	// ErrnoNomsg No message of the desired type.
	ErrnoNomsg
	// ErrnoNoprotoopt No message of the desired type.
	ErrnoNoprotoopt
	// ErrnoNospc No space left on device.
	ErrnoNospc
	// ErrnoNosys Function not supported.
	ErrnoNosys
	// ErrnoNotconn The socket is not connected.
	ErrnoNotconn
	// ErrnoNotdir Not a directory or a symbolic link to a directory.
	ErrnoNotdir
	// ErrnoNotempty Directory not empty.
	ErrnoNotempty
	// ErrnoNotrecoverable State not recoverable.
	ErrnoNotrecoverable
	// ErrnoNotsock Not a socket.
	ErrnoNotsock
	// ErrnoNotsup Not supported, or operation not supported on socket.
	ErrnoNotsup
	// ErrnoNotty Inappropriate I/O control operation.
	ErrnoNotty
	// ErrnoNxio No such device or address.
	ErrnoNxio
	// ErrnoOverflow Value too large to be stored in data type.
	ErrnoOverflow
	// ErrnoOwnerdead Previous owner died.
	ErrnoOwnerdead
	// ErrnoPerm Operation not permitted.
	ErrnoPerm
	// ErrnoPipe Broken pipe.
	ErrnoPipe
	// ErrnoProto Protocol error.
	ErrnoProto
	// ErrnoProtonosupport Protocol error.
	ErrnoProtonosupport
	// ErrnoPrototype Protocol wrong type for socket.
	ErrnoPrototype
	// ErrnoRange Result too large.
	ErrnoRange
	// ErrnoRofs Read-only file system.
	ErrnoRofs
	// ErrnoSpipe Invalid seek.
	ErrnoSpipe
	// ErrnoSrch No such process.
	ErrnoSrch
	// ErrnoStale Reserved.
	ErrnoStale
	// ErrnoTimedout Connection timed out.
	ErrnoTimedout
	// ErrnoTxtbsy Text file busy.
	ErrnoTxtbsy
	// ErrnoXdev Cross-device link.
	ErrnoXdev
	// ErrnoNotcapable Extension: Capabilities insufficient.
	ErrnoNotcapable
)

var errnoToString = [...]string{
	ErrnoSuccess:        "ESUCCESS",
	Errno2big:           "E2BIG",
	ErrnoAcces:          "EACCES",
	ErrnoAddrinuse:      "EADDRINUSE",
	ErrnoAddrnotavail:   "EADDRNOTAVAIL",
	ErrnoAfnosupport:    "EAFNOSUPPORT",
	ErrnoAgain:          "EAGAIN",
	ErrnoAlready:        "EALREADY",
	ErrnoBadf:           "EBADF",
	ErrnoBadmsg:         "EBADMSG",
	ErrnoBusy:           "EBUSY",
	ErrnoCanceled:       "ECANCELED",
	ErrnoChild:          "ECHILD",
	ErrnoConnaborted:    "ECONNABORTED",
	ErrnoConnrefused:    "ECONNREFUSED",
	ErrnoConnreset:      "ECONNRESET",
	ErrnoDeadlk:         "EDEADLK",
	ErrnoDestaddrreq:    "EDESTADDRREQ",
	ErrnoDom:            "EDOM",
	ErrnoDquot:          "EDQUOT",
	ErrnoExist:          "EEXIST",
	ErrnoFault:          "EFAULT",
	ErrnoFbig:           "EFBIG",
	ErrnoHostunreach:    "EHOSTUNREACH",
	ErrnoIdrm:           "EIDRM",
	ErrnoIlseq:          "EILSEQ",
	ErrnoInprogress:     "EINPROGRESS",
	ErrnoIntr:           "EINTR",
	ErrnoInval:          "EINVAL",
	ErrnoIo:             "EIO",
	ErrnoIsconn:         "EISCONN",
	ErrnoIsdir:          "EISDIR",
	ErrnoLoop:           "ELOOP",
	ErrnoMfile:          "EMFILE",
	ErrnoMlink:          "EMLINK",
	ErrnoMsgsize:        "EMSGSIZE",
	ErrnoMultihop:       "EMULTIHOP",
	ErrnoNametoolong:    "ENAMETOOLONG",
	ErrnoNetdown:        "ENETDOWN",
	ErrnoNetreset:       "ENETRESET",
	ErrnoNetunreach:     "ENETUNREACH",
	ErrnoNfile:          "ENFILE",
	ErrnoNobufs:         "ENOBUFS",
	ErrnoNodev:          "ENODEV",
	ErrnoNoent:          "ENOENT",
	ErrnoNoexec:         "ENOEXEC",
	ErrnoNolck:          "ENOLCK",
	ErrnoNolink:         "ENOLINK",
	ErrnoNomem:          "ENOMEM",
	ErrnoNomsg:          "ENOMSG",
	ErrnoNoprotoopt:     "ENOPROTOOPT",
	ErrnoNospc:          "ENOSPC",
	ErrnoNosys:          "ENOSYS",
	ErrnoNotconn:        "ENOTCONN",
	ErrnoNotdir:         "ENOTDIR",
	ErrnoNotempty:       "ENOTEMPTY",
	ErrnoNotrecoverable: "ENOTRECOVERABLE",
	ErrnoNotsock:        "ENOTSOCK",
	ErrnoNotsup:         "ENOTSUP",
	ErrnoNotty:          "ENOTTY",
	ErrnoNxio:           "ENXIO",
	ErrnoOverflow:       "EOVERFLOW",
	ErrnoOwnerdead:      "EOWNERDEAD",
	ErrnoPerm:           "EPERM",
	ErrnoPipe:           "EPIPE",
	ErrnoProto:          "EPROTO",
	ErrnoProtonosupport: "EPROTONOSUPPORT",
	ErrnoPrototype:      "EPROTOTYPE",
	ErrnoRange:          "ERANGE",
	ErrnoRofs:           "EROFS",
	ErrnoSpipe:          "ESPIPE",
	ErrnoSrch:           "ESRCH",
	ErrnoStale:          "ESTALE",
	ErrnoTimedout:       "ETIMEDOUT",
	ErrnoTxtbsy:         "ETXTBSY",
	ErrnoXdev:           "EXDEV",
	ErrnoNotcapable:     "ENOTCAPABLE",
}

const (
	// FunctionArgsGet reads command-line argument data.
	//
	// See ImportArgsGet
	// See API.ArgsGet
	// See FunctionArgsSizesGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-args_getargv-pointerpointeru8-argv_buf-pointeru8---errno
	FunctionArgsGet = "args_get"

	// ImportArgsGet is the WebAssembly 1.0 (MVP) Text format import of FunctionArgsGet
	ImportArgsGet = `(import "wasi_snapshot_preview1" "args_get"
    (func $wasi.args_get (param $argv i32) (param $argv_buf i32) (result (;errno;) i32)))`

	// FunctionArgsSizesGet returns command-line argument data sizes.
	//
	// See ImportArgsSizesGet
	// See API.ArgsSizesGet
	// See FunctionArgsGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-args_sizes_get---errno-size-size
	FunctionArgsSizesGet = "args_sizes_get"

	// ImportArgsSizesGet is the WebAssembly 1.0 (MVP) Text format import of FunctionArgsSizesGet
	ImportArgsSizesGet = `(import "wasi_snapshot_preview1" "args_sizes_get"
    (func $wasi.args_sizes_get (param $result.argc i32) (param $result.argv_buf_size i32) (result (;errno;) i32)))`

	// FunctionEnvironGet reads environment variable data.
	//
	// See ImportEnvironGet
	// See API.EnvironGet
	// See FunctionEnvironSizesGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-environ_getenviron-pointerpointeru8-environ_buf-pointeru8---errno
	FunctionEnvironGet = "environ_get"

	// ImportEnvironGet is the WebAssembly 1.0 (MVP) Text format import of FunctionEnvironGet
	ImportEnvironGet = `(import "wasi_snapshot_preview1" "environ_get"
    (func $wasi.environ_get (param $environ i32) (param $environ_buf i32) (result (;errno;) i32)))`

	// FunctionEnvironSizesGet returns environment variable data sizes.
	//
	// See ImportEnvironSizesGet
	// See API.EnvironSizesGet
	// See FunctionEnvironGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-environ_sizes_get---errno-size-size
	FunctionEnvironSizesGet = "environ_sizes_get"

	// ImportEnvironSizesGet is the WebAssembly 1.0 (MVP) Text format import of FunctionEnvironSizesGet
	ImportEnvironSizesGet = `
(import "wasi_snapshot_preview1" "environ_sizes_get"
    (func $wasi.environ_sizes_get (param $result.environc i32) (param $result.environBufSize i32) (result (;errno;) i32)))`

	// FunctionClockResGet returns the resolution of a clock.
	//
	// See ImportClockResGet
	// See API.ClockResGet
	// See FunctionClockTimeGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_res_getid-clockid---errno-timestamp
	FunctionClockResGet = "clock_res_get"

	// ImportClockResGet is the WebAssembly 1.0 (MVP) Text format import of FunctionClockResGet
	ImportClockResGet = `
(import "wasi_snapshot_preview1" "clock_res_get"
    (func $wasi.clock_res_get (param $id i32) (param $result.resolution i32) (result (;errno;) i32)))`

	// FunctionClockTimeGet returns the time value of a clock.
	//
	// See ImportClockTimeGet
	// See API.ClockResGet
	// See FunctionClockResGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_time_getid-clockid-precision-timestamp---errno-timestamp
	FunctionClockTimeGet = "clock_time_get"

	// ImportClockTimeGet is the WebAssembly 1.0 (MVP) Text format import of FunctionClockTimeGet
	ImportClockTimeGet = `(import "wasi_snapshot_preview1" "clock_time_get"
    (func $wasi.clock_time_get (param $id i32) (param $precision i64) (param $result.timestamp i32) (result (;errno;) i32)))`

	FunctionFdAdvise             = "fd_advise"
	FunctionFdAllocate           = "fd_allocate"
	FunctionFdClose              = "fd_close"
	FunctionFdDataSync           = "fd_datasync"
	FunctionFdFdstatGet          = "fd_fdstat_get"
	FunctionFdFdstatSetFlags     = "fd_fdstat_set_flags"
	FunctionFdFdstatSetRights    = "fd_fdstat_set_rights"
	FunctionFdFilestatGet        = "fd_filestat_get"
	FunctionFdFilestatSetSize    = "fd_filestat_set_size"
	FunctionFdFilestatSetTimes   = "fd_filestat_set_times"
	FunctionFdPread              = "fd_pread"
	FunctionFdPrestatGet         = "fd_prestat_get"
	FunctionFdPrestatDirName     = "fd_prestat_dir_name"
	FunctionFdPwrite             = "fd_pwrite"
	FunctionFdRead               = "fd_read"
	FunctionFdReaddir            = "fd_readdir"
	FunctionFdRenumber           = "fd_renumber"
	FunctionFdSeek               = "fd_seek"
	FunctionFdSync               = "fd_sync"
	FunctionFdTell               = "fd_tell"
	FunctionFdWrite              = "fd_write"
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
	
	// FunctionRandomGet write random data in buffer
	//
	// See ImportRandomGet
	// See API.RandomGet
	// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-random_getbuf-pointeru8-buf_len-size---errno
	FunctionRandomGet            = "random_get"
	
	// ImportRandomGet is the WebAssembly 1.0 (MVP) Text format import of FunctionRandomGet
	ImportRandomGet = `(import "wasi_snapshot_preview1" "random_get"
    (func $wasi.random_get (param $buf i32) (param $buf_len i32) (result (;errno;) i32)))`

	FunctionSockRecv             = "sock_recv"
	FunctionSockSend             = "sock_send"
	FunctionSockShutdown         = "sock_shutdown"
)
