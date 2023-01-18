package syscallfs

import (
	"fmt"
	"io/fs"
	"os"
	pathutil "path"
)

// Adapt adapts the input to FS unless it is already one. NewDirFS should be
// used instead, if the input is os.DirFS.
//
// Note: This performs no flag verification on FS.OpenFile. fs.FS cannot read
// flags as there is no parameter to pass them through with. Moreover, fs.FS
// documentation does not require the file to be present. In summary, we can't
// enforce flag behavior.
func Adapt(fs fs.FS, guestDir string) FS {
	if sys, ok := fs.(FS); ok {
		return sys
	}
	return &adapter{fs: fs, guestDir: guestDir}
}

type adapter struct {
	UnimplementedFS
	fs       fs.FS
	guestDir string
}

// String implements fmt.Stringer
func (a *adapter) String() string {
	return fmt.Sprintf("%v:%s:ro", a.fs, a.guestDir)
}

// GuestDir implements FS.GuestDir
func (a *adapter) GuestDir() string {
	return a.guestDir
}

// OpenFile implements FS.OpenFile
func (a *adapter) OpenFile(path string, flag int, perm fs.FileMode) (fs.File, error) {
	path = cleanPath(path)
	f, err := a.fs.Open(path)

	if err != nil {
		return nil, unwrapPathError(err)
	} else if osF, ok := f.(*os.File); ok {
		// If this is an OS file, it has same portability issues as dirFS.
		return maybeWrapFile(osF), nil
	}
	return f, nil
}

func cleanPath(name string) string {
	if len(name) == 0 {
		return name
	}
	// fs.ValidFile cannot be rooted (start with '/')
	cleaned := name
	if name[0] == '/' {
		cleaned = name[1:]
	}
	cleaned = pathutil.Clean(cleaned) // e.g. "sub/." -> "sub"
	return cleaned
}
