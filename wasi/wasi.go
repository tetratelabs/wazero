// Package wasi includes constants and interfaces used by both public and internal APIs.
package wasi

import (
	"fmt"
)

const (
	// ModuleSnapshotPreview1 is the module name WASI functions are exported into
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md
	ModuleSnapshotPreview1 = "wasi_snapshot_preview1"
)

// Errno are the error codes returned by WASI functions.
//
// Note: This is not always an error, as ErrnoSuccess is a valid code.
// Note: Codes are defined even when not relevant to WASI for use in higher-level libraries or alignment with POSIX.
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-errno-enumu16
// See https://linux.die.net/man/3/errno
type Errno = uint32 // alias for parity with internalwasm.ValueType

// ErrnoName returns the POSIX error code name, except ErrnoSuccess, which is not an error. Ex. Errno2big -> "E2BIG"
func ErrnoName(errno Errno) string {
	if int(errno) < len(errnoToString) {
		return errnoToString[errno]
	}
	return fmt.Sprintf("errno(%d)", errno)
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
	// ErrnoNosys ExportedFunction not supported.
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

// ExitCode is an arbitrary uint32 number returned by proc_exit.
// An exit code of 0 indicates successful termination. The meanings of other values are not defined by WASI.
//
// In wazero, if ProcExit is called, the calling function returns immediately, returning the given exit code as the error.
// You can get the exit code by casting the error as follows.
//
//   wasmFunction := m.ExportedFunction(/* omitted */)  // Some function which may call proc_exit
//   err := wasmFunction()
//   var exitCode wasi.ExitCode
//   if errors.As(err, &exitCode) {
//   	// The function is terminated by proc_exit.
//   }
//
// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#exitcode
// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#proc_exit
type ExitCode uint32

func (err ExitCode) Error() string {
	return fmt.Sprintf("terminated by proc_exit(%d)", uint32(err))
}
