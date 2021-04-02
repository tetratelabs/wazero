package wasi

import (
	"bytes"
	"io/fs"
	"os"
	"runtime"
	"strings"
)

const (
	// WASI open flags
	O_CREATE = 1 << iota
	O_DIR
	O_EXCL
	O_TRUNC

	// WASI fs rights
	R_FD_READ = 1 << iota
	R_FD_SEEK
	R_FD_FDSTAT_SET_FLAGS
	R_FD_SYNC
	R_FD_TELL
	R_FD_WRITE
)

type File interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
}

// FS is an interface for a preopened directory.
type FS interface {
	// OpenWASI is a general method to open a file, similar to
	// os.OpenFile, but with WASI flags and rights instead of POSIX.
	OpenWASI(dirFlags uint32, path string, oFlags uint32, fsRights, fsRightsInheriting uint64, fdFlags uint32) (File, error)
}

type dirFS string

// DirFS returns a file system (a wasi.FS) for the tree of files rooted at
// the directory dir. It's similar to os.DirFS, except that it implements
// wasi.FS instead of the fs.FS interface.
func DirFS(dir string) FS {
	return dirFS(dir)
}

func posixOpenFlags(oFlags uint32, fsRights uint64) (pFlags int) {
	if fsRights&R_FD_WRITE != 0 {
		pFlags |= os.O_RDWR
	}
	if oFlags&O_CREATE != 0 {
		pFlags |= os.O_CREATE
	}
	if oFlags&O_EXCL != 0 {
		pFlags |= os.O_EXCL
	}
	if oFlags&O_TRUNC != 0 {
		pFlags |= os.O_TRUNC
	}
	return
}

func (dir dirFS) OpenWASI(dirFlags uint32, path string, oFlags uint32, fsRights, fsRightsInheriting uint64, fdFlags uint32) (File, error) {
	// I'm not sure how to use all these passed flags and rights yet
	if !fs.ValidPath(path) || runtime.GOOS == "windows" && strings.IndexAny(path, `\:`) >= 0 {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrInvalid}
	}
	mode := fs.FileMode(0644)
	if oFlags&O_DIR != 0 {
		mode |= fs.ModeDir
	}
	f, err := os.OpenFile(string(dir)+"/"+path, posixOpenFlags(oFlags, fsRights), mode)
	if err != nil {
		return nil, err
	}
	return f, nil
}

type memFS struct {
	files map[string][]byte
}

func MemFS() FS {
	return &memFS{
		files: map[string][]byte{},
	}
}

func (m *memFS) OpenWASI(dirFlags uint32, path string, oFlags uint32, fsRights, fsRightsInheriting uint64, fdFlags uint32) (File, error) {
	if !fs.ValidPath(path) {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrInvalid}
	}

	var buf []byte
	if oFlags&O_CREATE == 0 {
		bts, ok := m.files[path]
		if !ok {
			return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
		}

		if oFlags&O_TRUNC == 0 {
			buf = append(buf, bts...)
		}
	}

	ret := &memFile{buf: bytes.NewBuffer(buf)}

	if fsRights&R_FD_WRITE != 0 {
		ret.flush = func(bts []byte) {
			m.files[path] = bts
		}
	}

	return ret, nil
}

type memFile struct {
	buf   *bytes.Buffer
	flush func(bts []byte)
}

func (f *memFile) Read(p []byte) (int, error) {
	return f.buf.Read(p)
}

func (f *memFile) Write(p []byte) (int, error) {
	return f.buf.Write(p)
}

func (f *memFile) Close() error {
	if f.flush != nil {
		f.flush(f.buf.Bytes())
	}
	return nil
}
