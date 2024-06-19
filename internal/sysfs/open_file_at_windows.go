package sysfs

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/tetratelabs/wazero/experimental/sys"
)

func openFileAt(dir *os.File, path_ string, oflag sys.Oflag, perm fs.FileMode) (*os.File, sys.Errno) {
	// Since `NtCreateFile` doesn't resolve `.` and `..`, we need to do that.
	// This implementation is adapted from cap-std's implementation of `openat`
	// on Windows also using `NtCreateFile``.
	// See https://github.com/bytecodealliance/cap-std/issues/226

	sep := string(filepath.Separator)
	path_ = strings.ReplaceAll(path_, "/", sep)
	rebuiltPath := ""

	for _, component := range strings.Split(path_, sep) {
		switch component {
		case "":
		case ".":
		case "..":
			if len(rebuiltPath) == 0 {
				rebuiltPath = dir.Name()
			}

			// And then pop the last component.
			rebuiltPath, _ = filepath.Split(rebuiltPath)
		default:
			rebuiltPath = filepath.Join(rebuiltPath, component)
		}
	}

	isDir := oflag&sys.O_DIRECTORY > 0
	flag := toOsOpenFlag(oflag)

	if oflag&sys.O_NOFOLLOW == sys.O_NOFOLLOW {
		flag |= int(sys.O_NOFOLLOW)
	}

	fd, err := openAt(syscall.Handle(dir.Fd()), rebuiltPath, flag|syscall.O_CLOEXEC, uint32(perm))
	if err != nil {
		errno := sys.UnwrapOSError(err)

		switch errno {
		case sys.EINVAL:
			// WASI expects ENOTDIR for a file path with a trailing slash.
			if strings.HasSuffix(path_, sep) {
				errno = sys.ENOTDIR
			}
		// To match expectations of WASI, e.g. TinyGo TestStatBadDir, return
		// ENOENT, not ENOTDIR.
		case sys.ENOTDIR:
			errno = sys.ENOENT
		case sys.ENOENT:
			if isSymlink(path_) {
				// Either symlink or hard link not found. We change the returned
				// errno depending on if it is symlink or not to have consistent
				// behavior across OSes.
				if isDir {
					// Dangling symlink dir must raise ENOTDIR.
					errno = sys.ENOTDIR
				} else {
					errno = sys.ELOOP
				}
			}
		}

		return nil, errno
	}

	return os.NewFile(uintptr(fd), path_), 0
}

// # Differences from syscall.Open
//
// This code is based on syscall.Open from the below link with some differences
// https://github.com/golang/go/blame/go1.20/src/syscall/syscall_windows.go#L308-L379
//
//   - syscall.O_CREAT doesn't imply syscall.GENERIC_WRITE as that breaks
//     flag expectations in wasi.
//   - add support for setting FILE_SHARE_DELETE.
func openAt(
	dirfd syscall.Handle,
	path string,
	mode int,
	perm uint32,
) (fd syscall.Handle, err error) {
	rootDirectory := syscall.Handle(0)
	if !filepath.IsAbs(path) {
		rootDirectory = dirfd
	} else {
		path = "\\??\\" + path
	}

	if len(path) == 0 {
		return syscall.InvalidHandle, syscall.ERROR_FILE_NOT_FOUND
	}

	objectName, err := NewNTUnicodeString(path)
	if err != nil {
		return syscall.InvalidHandle, err
	}

	var access uint32
	switch mode & (syscall.O_RDONLY | syscall.O_WRONLY | syscall.O_RDWR) {
	case syscall.O_RDONLY:
		access = FILE_GENERIC_READ
	case syscall.O_WRONLY:
		access = FILE_GENERIC_WRITE
	case syscall.O_RDWR:
		access = FILE_GENERIC_READ | FILE_GENERIC_WRITE
	}
	if mode&syscall.O_APPEND != 0 {
		access &^= syscall.GENERIC_WRITE
		access |= syscall.FILE_APPEND_DATA
	}

	sharemode := uint32(syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE | syscall.FILE_SHARE_DELETE)
	var sa *syscall.SecurityAttributes
	if mode&syscall.O_CLOEXEC == 0 {
		var _sa syscall.SecurityAttributes
		_sa.Length = uint32(unsafe.Sizeof(sa))
		_sa.InheritHandle = 1
		sa = &_sa
	}

	var disposition uint32
	switch {
	case mode&(syscall.O_CREAT|syscall.O_EXCL) == (syscall.O_CREAT | syscall.O_EXCL):
		disposition = FILE_CREATE
	case mode&(syscall.O_CREAT|syscall.O_TRUNC) == (syscall.O_CREAT | syscall.O_TRUNC):
		disposition = FILE_SUPERSEDE
	case mode&syscall.O_CREAT == syscall.O_CREAT:
		disposition = FILE_OPEN_IF
	case mode&syscall.O_TRUNC == syscall.O_TRUNC:
		disposition = FILE_OVERWRITE
	default:
		disposition = FILE_OPEN
	}

	objectAttributes := OBJECT_ATTRIBUTES{
		Length:             uint32(unsafe.Sizeof(OBJECT_ATTRIBUTES{})),
		RootDirectory:      rootDirectory,
		ObjectName:         objectName,
		Attributes:         OBJ_CASE_INSENSITIVE,
		SecurityDescriptor: 0,
		SecurityQoS:        0,
	}

	var (
		options        uint32
		allocationSize int64
		ioStatusBlock  IO_STATUS_BLOCK
		attrs          uint32 = syscall.FILE_ATTRIBUTE_NORMAL
	)

	// This shouldn't be included before 1.20 to have consistent behavior.
	// https://github.com/golang/go/commit/0f0aa5d8a6a0253627d58b3aa083b24a1091933f
	if disposition == FILE_OPEN && access == FILE_GENERIC_READ {
		// Necessary for opening directory handles.
		options |= FILE_OPEN_FOR_BACKUP_INTENT
	}

	if perm&syscall.S_IWRITE == 0 {
		attrs = syscall.FILE_ATTRIBUTE_READONLY

		if disposition == FILE_SUPERSEDE {
			// We have been asked to create a read-only file.
			// If the file already exists, the semantics of
			// the Unix open system call is to preserve the
			// existing permissions. If we pass FILE_SUPERSEDE
			// and FILE_ATTRIBUTE_READONLY to NtCreateFile,
			// and the file already exists, NtCreateFile will
			// change the file permissions.
			// Avoid that to preserve the Unix semantics.
			e := NtCreateFile(
				&fd,
				access,
				&objectAttributes,
				&ioStatusBlock,
				&allocationSize,
				syscall.FILE_ATTRIBUTE_NORMAL,
				sharemode,
				FILE_OVERWRITE,
				0,
				0,
				0,
			)
			switch e {
			case syscall.ERROR_FILE_NOT_FOUND, syscall.ERROR_PATH_NOT_FOUND:
				// File does not exist. These are the same
				// errors as Errno.Is checks for ErrNotExist.
				// Carry on to create the file.
			default:
				// Success or some different error.
				return fd, e
			}
		}
	}

	e := NtCreateFile(
		&fd,
		access,
		&objectAttributes,
		&ioStatusBlock,
		&allocationSize,
		attrs,
		sharemode,
		disposition,
		options,
		0,
		0,
	)
	if e != 0 {
		return syscall.InvalidHandle, e
	}

	if mode&int(sys.O_NOFOLLOW) == int(sys.O_NOFOLLOW) {
		// Emulate no follow behavior.
		var info syscall.ByHandleFileInformation

		err = syscall.GetFileInformationByHandle(fd, &info)
		if err != nil {
			return syscall.InvalidHandle, sys.UnwrapOSError(err)
		}

		if info.FileAttributes&syscall.FILE_ATTRIBUTE_ARCHIVE == syscall.FILE_ATTRIBUTE_ARCHIVE ||
			info.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT == syscall.FILE_ATTRIBUTE_REPARSE_POINT {
			return syscall.InvalidHandle, sys.ELOOP
		}
	}

	return fd, nil
}
