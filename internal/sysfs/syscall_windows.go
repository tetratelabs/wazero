package sysfs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"syscall"
)

// See https://learn.microsoft.com/en-us/windows/win32/debug/system-error-codes--0-499-
const (
	// ERROR_ACCESS_DENIED is a Windows error returned by syscall.Unlink
	// instead of syscall.EPERM
	ERROR_ACCESS_DENIED = syscall.Errno(5)

	// ERROR_INVALID_HANDLE is a Windows error returned by syscall.Write
	// instead of syscall.EBADF
	ERROR_INVALID_HANDLE = syscall.Errno(6)

	// ERROR_NEGATIVE_SEEK is a Windows error returned by os.Truncate
	// instead of syscall.EINVAL
	ERROR_NEGATIVE_SEEK = syscall.Errno(131)

	// ERROR_DIR_NOT_EMPTY is a Windows error returned by syscall.Rmdir
	// instead of syscall.ENOTEMPTY
	ERROR_DIR_NOT_EMPTY = syscall.Errno(145)

	// ERROR_ALREADY_EXISTS is a Windows error returned by os.Mkdir
	// instead of syscall.EEXIST
	ERROR_ALREADY_EXISTS = syscall.Errno(183)

	// ERROR_DIRECTORY is a Windows error returned by syscall.Rmdir
	// instead of syscall.ENOTDIR
	ERROR_DIRECTORY = syscall.Errno(267)
)

func adjustMkdirError(err error) error {
	if err == ERROR_ALREADY_EXISTS {
		return syscall.EEXIST
	}
	return err
}

func adjustRmdirError(err error) error {
	switch err {
	case ERROR_DIRECTORY:
		return syscall.ENOTDIR
	case ERROR_DIR_NOT_EMPTY:
		return syscall.ENOTEMPTY
	}
	return err
}

func adjustTruncateError(err error) error {
	if err == ERROR_NEGATIVE_SEEK {
		return syscall.EINVAL
	}
	return err
}

func adjustUnlinkError(err error) error {
	if err == ERROR_ACCESS_DENIED {
		return syscall.EISDIR
	}
	return err
}

// rename uses os.Rename as `windows.Rename` is internal in Go's source tree.
func rename(old, new string) (err error) {
	if err = os.Rename(old, new); err == nil {
		return
	}
	err = errors.Unwrap(err) // unwrap the link error
	if err == ERROR_ACCESS_DENIED {
		var newIsDir bool
		if stat, statErr := os.Stat(new); statErr == nil && stat.IsDir() {
			newIsDir = true
		}

		var oldIsDir bool
		if stat, statErr := os.Stat(old); statErr == nil && stat.IsDir() {
			oldIsDir = true
		}

		if oldIsDir && newIsDir {
			// Windows doesn't let you overwrite a directory. If we aim to
			// allow this, we'll have to delete here and retry.
			return syscall.EINVAL
		} else if newIsDir {
			err = syscall.EISDIR
		} else { // use a mappable code
			err = syscall.EPERM
		}
	}
	return
}

// maybeWrapFile deals with errno portability issues in Windows. This code is
// likely to change as we complete syscall support needed for WASI and GOOS=js.
//
// If we don't map to syscall.Errno, wasm will crash in odd way attempting the
// same. This approach is an alternative to making our own fs.File public type.
// We aren't doing that yet, as mapping problems are generally contained to
// Windows. Hence, file is intentionally not exported.
func maybeWrapFile(f file, fs FS, path string, flag int, perm fs.FileMode) file {
	return &windowsWrappedFile{f, f, f, f, f, f, fs, path, flag, perm, false}
}

type windowsWrappedFile struct {
	readFile
	io.Writer
	io.WriterAt // for pwrite
	syncer
	truncater
	fder
	fs                 FS
	path               string
	flag               int
	perm               fs.FileMode
	readDirInitialized bool
}

// ReadDir implements fs.ReadDirFile.
func (w *windowsWrappedFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if !w.readDirInitialized {
		if err := w.Close(); err != nil {
			return nil, err
		}
		newW, err := w.fs.OpenFile(w.path, w.flag, w.perm)
		if err != nil {
			return nil, err
		}

		*w = *newW.(*windowsWrappedFile)
		w.readDirInitialized = true
	}
	return w.readFile.ReadDir(n)
}

// Write implements io.Writer
func (w *windowsWrappedFile) Write(p []byte) (n int, err error) {
	n, err = w.Writer.Write(p)
	if err == nil {
		return
	}

	// os.File.Wrap wraps the syscall error in a path error
	if pe, ok := err.(*fs.PathError); ok {
		switch pe.Err {
		case ERROR_INVALID_HANDLE:
			pe.Err = syscall.EBADF
		case ERROR_ACCESS_DENIED:
			pe.Err = syscall.EPERM
		}
	}
	return
}
