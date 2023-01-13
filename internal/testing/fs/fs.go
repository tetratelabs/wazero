package testfs

import "io/fs"

// compile-time check to ensure File implements fs.File
var _ fs.File = &File{}

// compile-time check to ensure FS implements fs.FS
var _ fs.FS = &FS{}

// FS emulates fs.FS. Note: the path (map key) cannot begin with "/"!
type FS map[string]*File

// Open implements the same method as documented on fs.FS, except it doesn't
// validate the path.
func (f FS) Open(name string) (fs.File, error) {
	if file, ok := f[name]; !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	} else {
		return file, nil
	}
}

type File struct{ CloseErr error }

func (f *File) Close() error                       { return f.CloseErr }
func (f *File) Stat() (fs.FileInfo, error)         { return nil, nil }
func (f *File) Read(_ []byte) (int, error)         { return 0, nil }
func (f *File) Seek(_ int64, _ int) (int64, error) { return 0, nil }
