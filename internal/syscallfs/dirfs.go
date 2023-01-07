package syscallfs

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

func NewDirFS(dir string) (FS, error) {
	if dir == "" {
		panic("empty dir")
	}
	// For easier OS-specific concatenation later, append the path separator.
	dir = ensureTrailingPathSeparator(dir)
	if stat, err := os.Stat(dir); err != nil {
		return nil, syscall.ENOENT
	} else if !stat.IsDir() {
		return nil, syscall.ENOTDIR
	}
	return dirFS(dir), nil
}

func ensureTrailingPathSeparator(dir string) string {
	if dir[len(dir)-1] != os.PathSeparator {
		return dir + string(os.PathSeparator)
	}
	return dir
}

// dirFS currently validates each path, which means that input paths cannot
// escape the directory, except via symlink. We may want to relax this in the
// future, especially as we decoupled from fs.FS which has this requirement.
type dirFS string

// Open implements the same method as documented on fs.FS
func (dir dirFS) Open(name string) (fs.File, error) {
	panic(fmt.Errorf("unexpected to call fs.FS.Open(%s)", name))
}

// Path implements FS.Path
func (dir dirFS) Path() string {
	return "/"
}

// OpenFile implements FS.OpenFile
func (dir dirFS) OpenFile(name string, flag int, perm fs.FileMode) (fs.File, error) {
	f, err := os.OpenFile(dir.join(name), flag, perm)
	if err != nil {
		return nil, err
	}
	return maybeWrapFile(f), nil
}

// Mkdir implements FS.Mkdir
func (dir dirFS) Mkdir(name string, perm fs.FileMode) error {
	err := os.Mkdir(dir.join(name), perm)
	return adjustMkdirError(err)
}

// Rename implements FS.Rename
func (dir dirFS) Rename(from, to string) error {
	if from == to {
		return nil
	}
	return rename(dir.join(from), dir.join(to))
}

// Rmdir implements FS.Rmdir
func (dir dirFS) Rmdir(name string) error {
	err := syscall.Rmdir(dir.join(name))
	return adjustRmdirError(err)
}

// Unlink implements FS.Unlink
func (dir dirFS) Unlink(name string) error {
	err := syscall.Unlink(dir.join(name))
	return adjustUnlinkError(err)
}

// Utimes implements FS.Utimes
func (dir dirFS) Utimes(name string, atimeNsec, mtimeNsec int64) error {
	return syscall.UtimesNano(dir.join(name), []syscall.Timespec{
		syscall.NsecToTimespec(atimeNsec),
		syscall.NsecToTimespec(mtimeNsec),
	})
}

func (dir dirFS) join(name string) string {
	// TODO: Enforce similar to safefilepath.FromFS(name), but be careful as
	// relative path inputs are allowed. e.g. dir or name == ../
	return string(dir) + name
}
