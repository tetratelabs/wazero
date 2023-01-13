package syscallfs

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

func NewDirFS(guestDir, hostDir string) (FS, error) {
	// For easier OS-specific concatenation later, append the path separator.
	hostDir = ensureTrailingPathSeparator(hostDir)
	if stat, err := os.Stat(hostDir); err != nil {
		return nil, syscall.ENOENT
	} else if !stat.IsDir() {
		return nil, syscall.ENOTDIR
	}
	return &dirFS{guestDir, hostDir}, nil
}

func ensureTrailingPathSeparator(dir string) string {
	if dir[len(dir)-1] != os.PathSeparator {
		return dir + string(os.PathSeparator)
	}
	return dir
}

type dirFS struct {
	guestDir, hostDir string
}

// Open implements the same method as documented on fs.FS
func (d *dirFS) Open(name string) (fs.File, error) {
	panic(fmt.Errorf("unexpected to call fs.FS.Open(%s)", name))
}

// GuestDir implements FS.GuestDir
func (d *dirFS) GuestDir() string {
	return d.guestDir
}

// OpenFile implements FS.OpenFile
func (d *dirFS) OpenFile(name string, flag int, perm fs.FileMode) (fs.File, error) {
	f, err := os.OpenFile(d.join(name), flag, perm)
	if err != nil {
		return nil, err
	}
	return maybeWrapFile(f), nil
}

// Mkdir implements FS.Mkdir
func (d *dirFS) Mkdir(name string, perm fs.FileMode) error {
	err := os.Mkdir(d.join(name), perm)
	return adjustMkdirError(err)
}

// Rename implements FS.Rename
func (d *dirFS) Rename(from, to string) error {
	if from == to {
		return nil
	}
	return rename(d.join(from), d.join(to))
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

func (d *dirFS) join(name string) string {
	if name == "." {
		return d.hostDir
	}
	// TODO: Enforce similar to safefilepath.FromFS(name), but be careful as
	// relative path inputs are allowed. e.g. dir or name == ../
	return d.hostDir + name
}
