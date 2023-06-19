package sys

import (
	"io"
	"io/fs"
	"net"
	"syscall"

	"github.com/tetratelabs/wazero/internal/descriptor"
	"github.com/tetratelabs/wazero/internal/fsapi"
	"github.com/tetratelabs/wazero/internal/platform"
	socketapi "github.com/tetratelabs/wazero/internal/sock"
	"github.com/tetratelabs/wazero/internal/sysfs"
)

const (
	FdStdin int32 = iota
	FdStdout
	FdStderr
	// FdPreopen is the file descriptor of the first pre-opened directory.
	//
	// # Why file descriptor 3?
	//
	// While not specified, the most common WASI implementation, wasi-libc,
	// expects POSIX style file descriptor allocation, where the lowest
	// available number is used to open the next file. Since 1 and 2 are taken
	// by stdout and stderr, the next is 3.
	//   - https://github.com/WebAssembly/WASI/issues/122
	//   - https://pubs.opengroup.org/onlinepubs/9699919799/functions/V2_chap02.html#tag_15_14
	//   - https://github.com/WebAssembly/wasi-libc/blob/wasi-sdk-16/libc-bottom-half/sources/preopens.c#L215
	FdPreopen
)

const modeDevice = fs.ModeDevice | 0o640

// FileEntry maps a path to an open file in a file system.
type FileEntry struct {
	// Name is the name of the directory up to its pre-open, or the pre-open
	// name itself when IsPreopen.
	//
	// # Notes
	//
	//   - This can drift on rename.
	//   - This relates to the guest path, which is not the real file path
	//     except if the entire host filesystem was made available.
	Name string

	// IsPreopen is a directory that is lazily opened.
	IsPreopen bool

	// FS is the filesystem associated with the pre-open.
	FS fsapi.FS

	// File is always non-nil.
	File fsapi.File
}

type FSContext struct {
	// rootFS is the root ("/") mount.
	rootFS fsapi.FS

	// openedFiles is a map of file descriptor numbers (>=FdPreopen) to open files
	// (or directories) and defaults to empty.
	// TODO: This is unguarded, so not goroutine-safe!
	openedFiles FileTable

	// readdirs is a map of numeric identifiers to Readdir structs
	// and defaults to empty.
	// TODO: This is unguarded, so not goroutine-safe!
	readdirs ReaddirTable
}

// FileTable is a specialization of the descriptor.Table type used to map file
// descriptors to file entries.
type FileTable = descriptor.Table[int32, *FileEntry]

// ReaddirTable is a specialization of the descriptor.Table type used to map
// file descriptors to Readdir structs.
type ReaddirTable = descriptor.Table[int32, fsapi.Readdir]

// RootFS returns a possibly unimplemented root filesystem. Any files that
// should be added to the table should be inserted via InsertFile.
//
// TODO: This is only used by GOOS=js and tests: Remove when we remove GOOS=js
// (after Go 1.22 is released).
func (c *FSContext) RootFS() fsapi.FS {
	if rootFS := c.rootFS; rootFS == nil {
		return fsapi.UnimplementedFS{}
	} else {
		return rootFS
	}
}

// OpenFile opens the file into the table and returns its file descriptor.
// The result must be closed by CloseFile or Close.
func (c *FSContext) OpenFile(fs fsapi.FS, path string, flag int, perm fs.FileMode) (int32, syscall.Errno) {
	if f, errno := fs.OpenFile(path, flag, perm); errno != 0 {
		return 0, errno
	} else {
		fe := &FileEntry{FS: fs, File: f}
		if path == "/" || path == "." {
			fe.Name = ""
		} else {
			fe.Name = path
		}
		if newFD, ok := c.openedFiles.Insert(fe); !ok {
			return 0, syscall.EBADF
		} else {
			return newFD, 0
		}
	}
}

// SockAccept accepts a socketapi.TCPConn into the file table and returns
// its file descriptor.
func (c *FSContext) SockAccept(sockFD int32, nonblock bool) (int32, syscall.Errno) {
	var sock socketapi.TCPSock
	if e, ok := c.LookupFile(sockFD); !ok || !e.IsPreopen {
		return 0, syscall.EBADF // Not a preopen
	} else if sock, ok = e.File.(socketapi.TCPSock); !ok {
		return 0, syscall.EBADF // Not a sock
	}

	var conn socketapi.TCPConn
	var errno syscall.Errno
	if conn, errno = sock.Accept(); errno != 0 {
		return 0, errno
	} else if nonblock {
		if errno = conn.SetNonblock(true); errno != 0 {
			_ = conn.Close()
			return 0, errno
		}
	}

	fe := &FileEntry{File: conn}
	if newFD, ok := c.openedFiles.Insert(fe); !ok {
		return 0, syscall.EBADF
	} else {
		return newFD, 0
	}
}

// LookupFile returns a file if it is in the table.
func (c *FSContext) LookupFile(fd int32) (*FileEntry, bool) {
	return c.openedFiles.Lookup(fd)
}

// LookupReaddir returns a Readdir struct or creates an empty one if it was not present.
//
// Notes:
//   - this currently assumes that idx == fd, where fd is the file descriptor of the directory.
//     CloseFile will delete this idx from the internal store. In the future, idx may be independent
//     of a file fd, and the idx may have to be disposed with an explicit CloseReaddir.
//   - LookupReaddir is used in wasip1 fd_readdir. In wasip1 dot and dot-dot entries are required to exist,
//     but the reverse is true for preview2. Thus, when LookupReaddir retrieves a Readdir instance
//     via File.Readdir, we locally prepend dot entries to it and store that result.
func (c *FSContext) LookupReaddir(idx int32, f *FileEntry) (fsapi.Readdir, syscall.Errno) {
	if item, _ := c.readdirs.Lookup(idx); item != nil {
		return item, 0
	} else {
		readdir, err := f.File.Readdir()
		// Create a Readdir with "." and "..".
		// TODO: control over this needs to become a parameter to avoid adding dot entries
		// on wasip>1
		dotEntries, errno := synthesizeDotEntries(f)
		if errno != 0 {
			return nil, errno
		}
		// Prepend the dot-entries to the real directory listing.
		readdir = sysfs.NewConcatReaddir(sysfs.NewReaddir(dotEntries...), readdir)
		if err != 0 {
			return nil, err
		}
		ok := c.readdirs.InsertAt(readdir, idx)
		if !ok {
			return nil, syscall.EINVAL
		}
		return readdir, 0
	}
}

// synthesizeDotEntries generates a slice of the two elements "." and "..".
func synthesizeDotEntries(f *FileEntry) (result []fsapi.Dirent, errno syscall.Errno) {
	dotIno, errno := f.File.Ino()
	if errno != 0 {
		return nil, errno
	}
	result = append(result, fsapi.Dirent{Name: ".", Ino: dotIno, Type: fs.ModeDir})
	// See /RATIONALE.md for why we don't attempt to get an inode for ".."
	result = append(result, fsapi.Dirent{Name: "..", Ino: 0, Type: fs.ModeDir})
	return result, 0
}

// CloseReaddir delete the Readdir struct at the given index
//
// Note: Currently only necessary in tests. In the future, the idx will have to be disposed explicitly,
// unless we maintain a map fd -> []idx, and we let CloseFile close all the idx in []idx.
func (c *FSContext) CloseReaddir(idx int32) syscall.Errno {
	if item, ok := c.readdirs.Lookup(idx); ok {
		errno := item.Close()
		c.readdirs.Delete(idx)
		return errno
	} else {
		return syscall.EBADF
	}
}

// Renumber assigns the file pointed by the descriptor `from` to `to`.
func (c *FSContext) Renumber(from, to int32) syscall.Errno {
	fromFile, ok := c.openedFiles.Lookup(from)
	if !ok || to < 0 {
		return syscall.EBADF
	} else if fromFile.IsPreopen {
		return syscall.ENOTSUP
	}

	// If toFile is already open, we close it to prevent windows lock issues.
	//
	// The doc is unclear and other implementations do nothing for already-opened To FDs.
	// https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_renumberfd-fd-to-fd---errno
	// https://github.com/bytecodealliance/wasmtime/blob/main/crates/wasi-common/src/snapshots/preview_1.rs#L531-L546
	if toFile, ok := c.openedFiles.Lookup(to); ok {
		if toFile.IsPreopen {
			return syscall.ENOTSUP
		}
		_ = toFile.File.Close()
	}

	c.openedFiles.Delete(from)
	if !c.openedFiles.InsertAt(fromFile, to) {
		return syscall.EBADF
	}
	return 0
}

// CloseFile returns any error closing the existing file.
func (c *FSContext) CloseFile(fd int32) syscall.Errno {
	f, ok := c.openedFiles.Lookup(fd)
	if !ok {
		return syscall.EBADF
	}
	c.openedFiles.Delete(fd)
	_ = c.CloseReaddir(fd)
	return platform.UnwrapOSError(f.File.Close())
}

// Close implements io.Closer
func (c *FSContext) Close() (err error) {
	// Close any files opened in this context
	c.openedFiles.Range(func(fd int32, entry *FileEntry) bool {
		if errno := entry.File.Close(); errno != 0 {
			err = errno // This means err returned == the last non-nil error.
		}
		return true
	})
	// A closed FSContext cannot be reused so clear the state instead of
	// using Reset.
	c.openedFiles = FileTable{}
	c.readdirs = ReaddirTable{}
	return
}

// InitFSContext initializes a FSContext with stdio streams and optional
// pre-opened filesystems and TCP listeners.
func (c *Context) InitFSContext(
	stdin io.Reader,
	stdout, stderr io.Writer,
	fs []fsapi.FS, guestPaths []string,
	tcpListeners []*net.TCPListener,
) (err error) {
	inFile, err := stdinFileEntry(stdin)
	if err != nil {
		return err
	}
	c.fsc.openedFiles.Insert(inFile)
	outWriter, err := stdioWriterFileEntry("stdout", stdout)
	if err != nil {
		return err
	}
	c.fsc.openedFiles.Insert(outWriter)
	errWriter, err := stdioWriterFileEntry("stderr", stderr)
	if err != nil {
		return err
	}
	c.fsc.openedFiles.Insert(errWriter)

	for i, fs := range fs {
		guestPath := guestPaths[i]

		if StripPrefixesAndTrailingSlash(guestPath) == "" {
			c.fsc.rootFS = fs
		}
		c.fsc.openedFiles.Insert(&FileEntry{
			FS:        fs,
			Name:      guestPath,
			IsPreopen: true,
			File:      &lazyDir{fs: fs},
		})
	}

	for _, tl := range tcpListeners {
		c.fsc.openedFiles.Insert(&FileEntry{IsPreopen: true, File: sysfs.NewTCPListenerFile(tl)})
	}
	return nil
}

// StripPrefixesAndTrailingSlash skips any leading "./" or "/" such that the
// result index begins with another string. A result of "." coerces to the
// empty string "" because the current directory is handled by the guest.
//
// Results are the offset/len pair which is an optimization to avoid re-slicing
// overhead, as this function is called for every path operation.
//
// Note: Relative paths should be handled by the guest, as that's what knows
// what the current directory is. However, paths that escape the current
// directory e.g. "../.." have been found in `tinygo test` and this
// implementation takes care to avoid it.
func StripPrefixesAndTrailingSlash(path string) string {
	// strip trailing slashes
	pathLen := len(path)
	for ; pathLen > 0 && path[pathLen-1] == '/'; pathLen-- {
	}

	pathI := 0
loop:
	for pathI < pathLen {
		switch path[pathI] {
		case '/':
			pathI++
		case '.':
			nextI := pathI + 1
			if nextI < pathLen && path[nextI] == '/' {
				pathI = nextI + 1
			} else if nextI == pathLen {
				pathI = nextI
			} else {
				break loop
			}
		default:
			break loop
		}
	}
	return path[pathI:pathLen]
}
