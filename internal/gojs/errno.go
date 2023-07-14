package gojs

import (
	"io"

	"github.com/tetratelabs/wazero/experimental/sys"
)

// Errno is a (GOARCH=wasm) error, which must match a key in mapJSError.
//
// See https://github.com/golang/go/blob/go1.20/src/syscall/tables_js.go#L371-L494
type Errno struct {
	s string
}

// Error implements error.
func (e *Errno) Error() string {
	return e.s
}

// This order match constants from wasi_snapshot_preview1.ErrnoSuccess for
// easier maintenance.
var (
	// ErrnoAcces Permission denied.
	ErrnoAcces = &Errno{"EACCES"}
	// ErrnoAgain Resource unavailable, or operation would block.
	ErrnoAgain = &Errno{"EAGAIN"}
	// ErrnoBadf Bad file descriptor.
	ErrnoBadf = &Errno{"EBADF"}
	// ErrnoExist File exists.
	ErrnoExist = &Errno{"EEXIST"}
	// ErrnoFault Bad address.
	ErrnoFault = &Errno{"EFAULT"}
	// ErrnoIntr Interrupted function.
	ErrnoIntr = &Errno{"EINTR"}
	// ErrnoInval Invalid argument.
	ErrnoInval = &Errno{"EINVAL"}
	// ErrnoIo I/O error.
	ErrnoIo = &Errno{"EIO"}
	// ErrnoIsdir Is a directory.
	ErrnoIsdir = &Errno{"EISDIR"}
	// ErrnoLoop Too many levels of symbolic links.
	ErrnoLoop = &Errno{"ELOOP"}
	// ErrnoNametoolong Filename too long.
	ErrnoNametoolong = &Errno{"ENAMETOOLONG"}
	// ErrnoNoent No such file or directory.
	ErrnoNoent = &Errno{"ENOENT"}
	// ErrnoNosys function not supported.
	ErrnoNosys = &Errno{"ENOSYS"}
	// ErrnoNotdir Not a directory or a symbolic link to a directory.
	ErrnoNotdir = &Errno{"ENOTDIR"}
	// ErrnoNotempty Directory not empty.
	ErrnoNotempty = &Errno{"ENOTEMPTY"}
	// ErrnoNotsup Not supported, or operation not supported on socket.
	ErrnoNotsup = &Errno{"ENOTSUP"}
	// ErrnoPerm Operation not permitted.
	ErrnoPerm = &Errno{"EPERM"}
	// ErrnoRofs read-only file system.
	ErrnoRofs = &Errno{"EROFS"}
)

// ToErrno maps I/O errors as the message must be the code, ex. "EINVAL", not
// the message, e.g. "invalid argument".
func ToErrno(err error) *Errno {
	if err == nil || err == io.EOF {
		return nil // io.EOF has no value in GOOS=js, and isn't an error.
	}
	errno, ok := err.(sys.Errno)
	if !ok {
		return ErrnoIo
	}
	switch errno {
	case sys.EACCES:
		return ErrnoAcces
	case sys.EAGAIN:
		return ErrnoAgain
	case sys.EBADF:
		return ErrnoBadf
	case sys.EEXIST:
		return ErrnoExist
	case sys.EFAULT:
		return ErrnoFault
	case sys.EINTR:
		return ErrnoIntr
	case sys.EINVAL:
		return ErrnoInval
	case sys.EIO:
		return ErrnoIo
	case sys.EISDIR:
		return ErrnoIsdir
	case sys.ELOOP:
		return ErrnoLoop
	case sys.ENAMETOOLONG:
		return ErrnoNametoolong
	case sys.ENOENT:
		return ErrnoNoent
	case sys.ENOSYS:
		return ErrnoNosys
	case sys.ENOTDIR:
		return ErrnoNotdir
	case sys.ENOTEMPTY:
		return ErrnoNotempty
	case sys.ENOTSUP:
		return ErrnoNotsup
	case sys.EPERM:
		return ErrnoPerm
	case sys.EROFS:
		return ErrnoRofs
	default:
		return ErrnoIo
	}
}
