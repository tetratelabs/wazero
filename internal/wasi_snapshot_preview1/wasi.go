// Package wasi_snapshot_preview1 is an internal helper to remove package
// cycles re-using errno
package wasi_snapshot_preview1

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/logging"
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

func ValueLoggers(fnd api.FunctionDefinition) (pLoggers []logging.ParamLogger, rLoggers []logging.ResultLogger) {
	pLoggers, rLoggers = logging.ValueLoggers(fnd)

	// All WASI functions except proc_after return only an errno result.
	if fnd.Name() == "proc_exit" {
		return logging.ValueLoggers(fnd)
	}
	rLoggers[0] = wasiErrno
	return
}

func wasiErrno(_ context.Context, _ api.Module, w logging.Writer, _, results []uint64) {
	errno := ErrnoName(uint32(results[0]))
	w.WriteString(errno) // nolint
}
