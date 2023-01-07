package syscallfs

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"syscall"
)

// NewReadFS is used to mask an existing FS for reads. Notably, this allows
// the CLI to do read-only mounts of directories the host user can write, but
// doesn't want the guest wasm to. For example, Python libraries shouldn't be
// written to at runtime by the python wasm file.
func NewReadFS(fs FS) FS {
	if _, ok := fs.(*readFS); ok {
		return fs
	}
	return &readFS{fs}
}

type readFS struct{ fs FS }

// Open implements the same method as documented on fs.FS
func (r *readFS) Open(name string) (fs.File, error) {
	panic(fmt.Errorf("unexpected to call fs.FS.Open(%s)", name))
}

// Path implements FS.Path
func (r *readFS) Path() string {
	return "/"
}

// OpenFile implements FS.OpenFile
func (r *readFS) OpenFile(path string, flag int, perm fs.FileMode) (fs.File, error) {
	if flag != 0 && flag != os.O_RDONLY {
		return nil, syscall.ENOSYS
	}

	f, err := r.fs.OpenFile(path, flag, perm)
	if err != nil {
		return nil, err
	}
	return maskForReads(f), nil
}

// maskForReads masks the file with read-only interfaces used by wazero.
//
// This technique was adapted from similar code in zipkin-go.
func maskForReads(f fs.File) fs.File {
	// The below are the types wazero casts into.
	// Note: os.File implements this even for normal files.
	d, i0 := f.(fs.ReadDirFile)
	ra, i1 := f.(io.ReaderAt)
	s, i2 := f.(io.Seeker)

	// Wrap any combination of the types above.
	switch {
	case !i0 && !i1 && !i2: // 0, 0, 0
		return struct{ fs.File }{f}
	case !i0 && !i1 && i2: // 0, 0, 1
		return struct {
			fs.File
			io.Seeker
		}{f, s}
	case !i0 && i1 && !i2: // 0, 1, 0
		return struct {
			fs.File
			io.ReaderAt
		}{f, ra}
	case !i0 && i1 && i2: // 0, 1, 1
		return struct {
			fs.File
			io.ReaderAt
			io.Seeker
		}{f, ra, s}
	case i0 && !i1 && !i2: // 1, 0, 0
		return struct {
			fs.ReadDirFile
		}{d}
	case i0 && !i1 && i2: // 1, 0, 1
		return struct {
			fs.ReadDirFile
			io.Seeker
		}{d, s}
	case i0 && i1 && !i2: // 1, 1, 0
		return struct {
			fs.ReadDirFile
			io.ReaderAt
		}{d, ra}
	case i0 && i1 && i2: // 1, 1, 1
		return struct {
			fs.ReadDirFile
			io.ReaderAt
			io.Seeker
		}{d, ra, s}
	default:
		panic("BUG: unhandled pattern")
	}
}

// Mkdir implements FS.Mkdir
func (r *readFS) Mkdir(path string, perm fs.FileMode) error {
	return syscall.ENOSYS
}

// Rename implements FS.Rename
func (r *readFS) Rename(from, to string) error {
	return syscall.ENOSYS
}

// Rmdir implements FS.Rmdir
func (r *readFS) Rmdir(path string) error {
	return syscall.ENOSYS
}

// Unlink implements FS.Unlink
func (r *readFS) Unlink(path string) error {
	return syscall.ENOSYS
}

// Utimes implements FS.Utimes
func (r *readFS) Utimes(path string, atimeNsec, mtimeNsec int64) error {
	return syscall.ENOSYS
}
