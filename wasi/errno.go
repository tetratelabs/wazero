package wasi

import "fmt"

// Errno is a type representing standard WASI error codes and implementing the
// error interface.
type Errno uint32

func (err Errno) Error() string {
	if int(err) < len(errnoToString) {
		return errnoToString[err]
	}
	return fmt.Sprintf("errno(%d)", uint32(err))
}

// WASI error codes
const (
	ESUCCESS        Errno = 0
	E2BIG           Errno = 1 /* Arg list too long */
	EACCES          Errno = 2 /* Permission denied */
	EADDRINUSE      Errno = 3 /* Address already in use */
	EADDRNOTAVAIL   Errno = 4 /* Cannot assign requested address */
	EAFNOSUPPORT    Errno = 5 /* Address family not supported by protocol */
	EAGAIN          Errno = 6 /* Try again */
	EALREADY        Errno = 7 /* Operation already in progress */
	EBADF           Errno = 8 /* Bad file number */
	EBADMSG         Errno = 9
	EBUSY           Errno = 10
	ECANCELED       Errno = 11
	ECHILD          Errno = 12
	ECONNABORTED    Errno = 13
	ECONNREFUSED    Errno = 14
	ECONNRESET      Errno = 15
	EDEADLK         Errno = 16
	EDESTADDRREQ    Errno = 17
	EDOM            Errno = 18
	EDQUOT          Errno = 19
	EEXIST          Errno = 20
	EFAULT          Errno = 21
	EFBIG           Errno = 22
	EHOSTUNREACH    Errno = 23
	EIDRM           Errno = 24
	EILSEQ          Errno = 25
	EINPROGRESS     Errno = 26
	EINTR           Errno = 27
	EINVAL          Errno = 28 /* Invalid argument */
	EIO             Errno = 29
	EISCONN         Errno = 30
	EISDIR          Errno = 31
	ELOOP           Errno = 32
	EMFILE          Errno = 33
	EMLINK          Errno = 34
	EMSGSIZE        Errno = 35
	EMULTIHOP       Errno = 36
	ENAMETOOLONG    Errno = 37
	ENETDOWN        Errno = 38
	ENETRESET       Errno = 39
	ENETUNREACH     Errno = 40
	ENFILE          Errno = 41
	ENOBUFS         Errno = 42
	ENODEV          Errno = 43
	ENOENT          Errno = 44
	ENOEXEC         Errno = 45
	ENOLCK          Errno = 46
	ENOLINK         Errno = 47
	ENOMEM          Errno = 48
	ENOMSG          Errno = 49
	ENOPROTOOPT     Errno = 50
	ENOSPC          Errno = 51
	ENOSYS          Errno = 52
	ENOTCONN        Errno = 53
	ENOTDIR         Errno = 54
	ENOTEMPTY       Errno = 55
	ENOTRECOVERABLE Errno = 56
	ENOTSOCK        Errno = 57
	ENOTSUP         Errno = 58
	ENOTTY          Errno = 59
	ENXIO           Errno = 60
	EOVERFLOW       Errno = 61
	EOWNERDEAD      Errno = 62
	EPERM           Errno = 63
	EPIPE           Errno = 64
	EPROTO          Errno = 65
	EPROTONOSUPPORT Errno = 66
	EPROTOTYPE      Errno = 67
	ERANGE          Errno = 68
	EROFS           Errno = 69
	ESPIPE          Errno = 70
	ESRCH           Errno = 71
	ESTALE          Errno = 72
	ETIMEDOUT       Errno = 73
	ETXTBSY         Errno = 74
	EXDEV           Errno = 75
	ENOTCAPABLE     Errno = 76
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
