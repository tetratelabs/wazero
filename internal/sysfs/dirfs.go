package sysfs

import (
	"io/fs"
	"os"
	"syscall"

	"github.com/tetratelabs/wazero/internal/platform"
)

func NewDirFS(dir string) FS {
	return &dirFS{
		dir:        dir,
		cleanedDir: ensureTrailingPathSeparator(dir),
	}
}

func ensureTrailingPathSeparator(dir string) string {
	if dir[len(dir)-1] != os.PathSeparator {
		return dir + string(os.PathSeparator)
	}
	return dir
}

type dirFS struct {
	UnimplementedFS
	dir string
	// cleanedDir is for easier OS-specific concatenation, as it always has
	// a trailing path separator.
	cleanedDir string
}

// String implements fmt.Stringer
func (d *dirFS) String() string {
	return d.dir
}

// Open implements the same method as documented on fs.FS
func (d *dirFS) Open(name string) (fs.File, error) {
	return fsOpen(d, name)
}

// OpenFile implements FS.OpenFile
func (d *dirFS) OpenFile(name string, flag int, perm fs.FileMode) (fs.File, error) {
	f, err := platform.OpenFile(d.join(name), flag, perm)
	if err != nil {
		return nil, unwrapOSError(err)
	}
	return maybeWrapFile(f), nil
}

// Mkdir implements FS.Mkdir
func (d *dirFS) Mkdir(name string, perm fs.FileMode) error {
	err := os.Mkdir(d.join(name), perm)
	err = unwrapOSError(err)
	return adjustMkdirError(err)
}

// Rename implements FS.Rename
func (d *dirFS) Rename(from, to string) error {
	from, to = d.join(from), d.join(to)
	return platform.Rename(from, to)
}

// Rmdir implements FS.Rmdir
func (d *dirFS) Rmdir(name string) error {
	err := syscall.Rmdir(d.join(name))
	return adjustRmdirError(err)
}

// Unlink implements FS.Unlink
func (d *dirFS) Unlink(name string) error {
	err := syscall.Unlink(d.join(name))
	return adjustUnlinkError(err)
}

// Utimes implements FS.Utimes
func (d *dirFS) Utimes(name string, atimeNsec, mtimeNsec int64) error {
	return syscall.UtimesNano(d.join(name), []syscall.Timespec{
		syscall.NsecToTimespec(atimeNsec),
		syscall.NsecToTimespec(mtimeNsec),
	})
}

// Truncate implements FS.Truncate
func (d *dirFS) Truncate(name string, size int64) error {
	// Use os.Truncate as syscall.Truncate doesn't exist on Windows.
	err := os.Truncate(d.join(name), size)
	err = unwrapOSError(err)
	return adjustTruncateError(err)
}

func (d *dirFS) join(name string) string {
	switch name {
	case "", ".", "/":
		// cleanedDir includes an unnecessary delimiter for the root path.
		return d.cleanedDir[:len(d.cleanedDir)-1]
	}
	// TODO: Enforce similar to safefilepath.FromFS(name), but be careful as
	// relative path inputs are allowed. e.g. dir or name == ../
	return d.cleanedDir + name
}
