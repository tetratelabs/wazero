// Package wasi_snapshot_preview1 is an internal helper to remove package
// cycles re-using errno
package wasi_snapshot_preview1

import (
	"context"
	"fmt"
	"strings"

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
	switch fnd.Name() {
	case "fd_prestat_get":
		pLoggers = []logging.ParamLogger{
			logging.NewParamLogger(0, "fd", logging.ValueTypeI32),
		}
		name := "prestat"
		rLoggers = []logging.ResultLogger{
			resultParamLogger(name, logging.NewParamLogger(1, name, logging.ValueTypeMemH64)),
			wasiErrno,
		}
		return
	case "proc_exit":
		return logging.ValueLoggers(fnd)
	}

	for idx := uint32(0); idx < uint32(len(fnd.ParamTypes())); idx++ {
		name := fnd.ParamNames()[idx]
		isResult := strings.HasPrefix(name, "result.")

		var logger logging.ParamLogger
		if strings.Contains(name, "path") {
			if isResult {
				name = name[7:]
			}
			logger = logging.NewParamLogger(idx, name, logging.ValueTypeString)
			idx++
			if isResult {
				rLoggers = append(rLoggers, resultParamLogger(name, logger))
				continue
			}
		} else if name == "result.nread" {
			name = name[7:]
			logger = logging.NewParamLogger(idx, name, logging.ValueTypeMemI32)
			rLoggers = append(rLoggers, resultParamLogger(name, logger))
			continue
		} else {
			logger = logging.NewParamLogger(idx, name, fnd.ParamTypes()[idx])
		}
		pLoggers = append(pLoggers, logger)
	}
	// All WASI functions except proc_after return only an errno result.
	rLoggers = append(rLoggers, wasiErrno)
	return
}

func wasiErrno(_ context.Context, _ api.Module, w logging.Writer, _, results []uint64) {
	errno := ErrnoName(uint32(results[0]))
	w.WriteString("errno=") // nolint
	w.WriteString(errno)    // nolint
}

// resultParamLogger logs the value of the parameter when the operation is
// successful or faults due to out of memory.
func resultParamLogger(name string, pLogger logging.ParamLogger) logging.ResultLogger {
	empty := name + "="
	return func(ctx context.Context, mod api.Module, w logging.Writer, params, results []uint64) {
		switch results[0] {
		case 0, 21: // ESUCCESS, EFAULT
			pLogger(ctx, mod, w, params)
		default:
			w.WriteString(empty) // nolint
		}
	}
}
