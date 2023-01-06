package syscallfs

import (
	"io/fs"
	"os"
	"path"
	"syscall"
)

func NewDirFS(dir string) (FS, error) {
	if stat, err := os.Stat(dir); err != nil {
		return nil, syscall.ENOENT
	} else if !stat.IsDir() {
		return nil, syscall.ENOTDIR
	}
	return dirFS(dir), nil
}

// dirFS currently validates each path, which means that input paths cannot
// escape the directory, except via symlink. We may want to relax this in the
// future, especially as we decoupled from fs.FS which has this requirement.
type dirFS string

// Open implements the same method as documented on fs.FS
func (dir dirFS) Open(name string) (fs.File, error) {
	return dir.OpenFile(name, 0, 0)
}

// Path implements FS.Path
func (dir dirFS) Path() string {
	return "/"
}

// OpenFile implements FS.OpenFile
func (dir dirFS) OpenFile(name string, flag int, perm fs.FileMode) (fs.File, error) {
	f, err := os.OpenFile(path.Join(string(dir), name), flag, perm)
	if err != nil {
		return nil, err
	}

	if flag == 0 || flag == os.O_RDONLY {
		return maskForReads(f), nil
	}
	return f, nil
}

// Mkdir implements FS.Mkdir
func (dir dirFS) Mkdir(name string, perm fs.FileMode) error {
	err := os.Mkdir(path.Join(string(dir), name), perm)
	return adjustMkdirError(err)
}

// Rename implements FS.Rename
func (dir dirFS) Rename(from, to string) error {
	if from == to {
		return nil
	}
	return rename(path.Join(string(dir), from), path.Join(string(dir), to))
}

// Rmdir implements FS.Rmdir
func (dir dirFS) Rmdir(name string) error {
	err := syscall.Rmdir(path.Join(string(dir), name))
	return adjustRmdirError(err)
}

// Unlink implements FS.Unlink
func (dir dirFS) Unlink(name string) error {
	err := syscall.Unlink(path.Join(string(dir), name))
	return adjustUnlinkError(err)
}

// Utimes implements FS.Utimes
func (dir dirFS) Utimes(name string, atimeNsec, mtimeNsec int64) error {
	return syscall.UtimesNano(path.Join(string(dir), name), []syscall.Timespec{
		syscall.NsecToTimespec(atimeNsec),
		syscall.NsecToTimespec(mtimeNsec),
	})
}
