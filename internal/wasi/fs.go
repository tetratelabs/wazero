package internalwasi

import (
	"bytes"
	"io/fs"
	"os"
	"runtime"
	"strings"

	"github.com/tetratelabs/wazero/wasi"
)

type DirFS string

func posixOpenFlags(oFlags uint32, fsRights uint64) (pFlags int) {
	if fsRights&wasi.R_FD_WRITE != 0 {
		pFlags |= os.O_RDWR
	}
	if oFlags&wasi.O_CREATE != 0 {
		pFlags |= os.O_CREATE
	}
	if oFlags&wasi.O_EXCL != 0 {
		pFlags |= os.O_EXCL
	}
	if oFlags&wasi.O_TRUNC != 0 {
		pFlags |= os.O_TRUNC
	}
	return
}

func (dir DirFS) OpenWASI(dirFlags uint32, path string, oFlags uint32, fsRights, fsRightsInheriting uint64, fdFlags uint32) (wasi.File, error) {
	// I'm not sure how to use all these passed flags and rights yet
	if !fs.ValidPath(path) || runtime.GOOS == "windows" && strings.ContainsAny(path, `\:`) {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrInvalid}
	}
	mode := fs.FileMode(0644)
	if oFlags&wasi.O_DIR != 0 {
		mode |= fs.ModeDir
	}
	f, err := os.OpenFile(string(dir)+"/"+path, posixOpenFlags(oFlags, fsRights), mode)
	if err != nil {
		return nil, err
	}
	return f, nil
}

type MemFS struct {
	Files map[string][]byte
}

func (m *MemFS) OpenWASI(dirFlags uint32, path string, oFlags uint32, fsRights, fsRightsInheriting uint64, fdFlags uint32) (wasi.File, error) {
	if !fs.ValidPath(path) {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrInvalid}
	}

	var buf []byte
	if oFlags&wasi.O_CREATE == 0 {
		bts, ok := m.Files[path]
		if !ok {
			return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
		}

		if oFlags&wasi.O_TRUNC == 0 {
			buf = append(buf, bts...)
		}
	}

	ret := &memFile{buf: bytes.NewBuffer(buf)}

	if fsRights&wasi.R_FD_WRITE != 0 {
		ret.flush = func(bts []byte) {
			m.Files[path] = bts
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
