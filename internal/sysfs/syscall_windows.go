package sysfs

import (
	"io/fs"
	"syscall"

	"github.com/tetratelabs/wazero/internal/platform"
)

// maybeWrapFile deals with errno portability issues in Windows. This code is
// likely to change as we complete syscall support needed for WASI and GOOS=js.
//
// If we don't map to syscall.Errno, wasm will crash in odd way attempting the
// same. This approach is an alternative to making our own fs.File public type.
// We aren't doing that yet, as mapping problems are generally contained to
// Windows. Hence, file is intentionally not exported.
func maybeWrapFile(f file, fs FS, path string, flag int, perm fs.FileMode) file {
	return &windowsWrappedFile{f, fs, path, flag, perm, false}
}

type windowsWrappedFile struct {
	file
	fs                 FS
	path               string
	flag               int
	perm               fs.FileMode
	readDirInitialized bool
}

// ReadDir implements fs.ReadDirFile.
func (w *windowsWrappedFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if !w.readDirInitialized {
		// On Windows, once the directory is opened, changes to the directory
		// is not visible on ReadDir on that already-opened file handle.
		//
		// In order to provide consistent behavior with other platforms, we re-open it.
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
	return w.file.ReadDir(n)
}

// Write implements io.Writer
func (w *windowsWrappedFile) Write(p []byte) (n int, err error) {
	n, err = w.file.Write(p)
	if err == nil {
		return
	}

	// os.File.Wrap wraps the syscall error in a path error
	if pe, ok := err.(*fs.PathError); ok {
		if pe.Err = platform.UnwrapOSError(pe.Err); pe.Err == syscall.EPERM {
			// go1.20 returns access denied, not invalid handle, writing to a directory.
			var stat platform.Stat_t
			if statErr := w.fs.Stat(w.path, &stat); statErr == nil && stat.Mode.IsDir() {
				pe.Err = syscall.EBADF
			}
		}
	}
	return
}
