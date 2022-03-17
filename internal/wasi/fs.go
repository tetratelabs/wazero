package internalwasi

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"runtime"
	"sort"
	"strings"
	"time"
)

// DirFS is a file system that implements fs.FS and wasi.OpenFileFS, similar to the read-only os.DirFS.
// This is the only implementation that allows the Wasm to create or change files on the host file system, since
// there is no official alternative interface to wasi.OpenFileFS.
// See os.DirFS.
type DirFS string // string holds the root path of this directory on the host file system.

// OpenFile implements wasi.OpenFileFS.OpenFile.
func (dir DirFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	if !fs.ValidPath(name) ||
		// '\' works as alternate path separater and ':' allows to express a root drive directory.
		// fs.FS implementation must reject those path on Windows. See Note in the doc of fs.ValidPath.
		runtime.GOOS == "windows" && strings.ContainsAny(name, `\:`) {
		return nil, &os.PathError{Op: "open", Path: name, Err: os.ErrInvalid}
	}
	return os.OpenFile(string(dir)+"/"+name, flag, perm)
}

// Open implements fs.FS.Open.
func (dir DirFS) Open(path string) (fs.File, error) {
	return dir.OpenFile(path, os.O_RDONLY, 0)
}

// MemFS is an in-memory file system that implements fs.FS and wasi.OpenFileFS.
// This is the only in-memory FS implementation that allows the Wasm to create or change files, since
// there is no official alternative interface to wasi.OpenFileFS.
type MemFS map[string]*memFSEntry

// OpenFile implements wasi.OpenFileFS.OpenFile.
func (m MemFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &os.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

	// Path "." indicates m itself as a directory.
	if name == "." {
		return &memDir{name: ".", fsEntry: &memFSEntry{Entries: m}}, nil
	}

	// Walk directories until we find the directory that the target file belongs to, updating `dir`.
	dir := &memFSEntry{Entries: m}
	files := strings.Split(name, "/")
	for _, file := range files[:len(files)-1] {
		var ok bool
		dir, ok = dir.Entries[file]
		if !ok || !dir.IsDir() {
			return nil, &os.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
	}
	// now `dir` is the directory that the target file belongs to.

	// Open or create the entry.
	// Note: MemFS does not check permission for now.
	baseName := path.Base(name)
	entry, ok := dir.Entries[baseName]
	if !ok {
		if flag&os.O_CREATE == 0 {
			return nil, &os.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}

		// Create a new entry.
		entry = &memFSEntry{}
		dir.Entries[baseName] = entry
	} else if flag&os.O_EXCL != 0 && flag&os.O_CREATE != 0 {
		// when O_EXCL is set with O_CREATE, the file must not already exist.
		return nil, &os.PathError{Op: "open", Path: name, Err: fs.ErrExist}
	}

	if flag&os.O_TRUNC != 0 {
		entry.Contents = []byte{}
	}

	if entry.IsDir() {
		return &memDir{name: baseName, fsEntry: entry}, nil
	} else {
		return &memFile{name: baseName, fsEntry: entry}, nil
	}
}

// Open implements fs.FS.Open.
func (m *MemFS) Open(path string) (fs.File, error) {
	return m.OpenFile(path, os.O_RDONLY, 0)
}

// memFSEntry represents a file or directory entry of memFS.
type memFSEntry struct {
	// Contents of the file if this file is a regular file.
	Contents []byte
	// Entries if this file is a directory. Otherwise, nil.
	Entries map[string]*memFSEntry
}

// IsDir returns if this entry is a directory.
func (e *memFSEntry) IsDir() bool { return e.Entries != nil }

// memFile represents an opened memFS regular file.
// memFile implements fs.File, io.Writer, and io.Seeker.
type memFile struct {
	// name of the opened file, not a full path but a base name.
	name string
	// Seek offset of this opened file.
	offset int64
	// The memFSEntry this memFile is opened for.
	fsEntry *memFSEntry
}

// Stat implements fs.File.Stat
func (f *memFile) Stat() (fs.FileInfo, error) {
	return &memFileInfo{name: f.name, fsEntry: f.fsEntry}, nil
}

// Read implements fs.File.Read
func (f *memFile) Read(p []byte) (int, error) {
	// In memFile, the end of the buffer is the end of the file.
	if f.offset == int64(len(f.fsEntry.Contents)) {
		return 0, io.EOF
	}
	nread := copy(p, f.fsEntry.Contents[f.offset:])
	f.offset += int64(nread)
	return nread, nil
}

// Close implements fs.File.Close
func (f *memFile) Close() error {
	f.fsEntry = nil
	return nil
}

// Write implements io.Writer
func (f *memFile) Write(p []byte) (int, error) {
	nwritten := copy(f.fsEntry.Contents[f.offset:], p)
	f.fsEntry.Contents = append(f.fsEntry.Contents, p[nwritten:]...)
	f.offset += int64(len(p))
	return len(p), nil
}

// Seek implements io.Seeker
func (f *memFile) Seek(offset int64, whence int) (int64, error) {
	var newOffset int64
	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset = f.offset + offset
	case io.SeekEnd:
		newOffset = int64(len(f.fsEntry.Contents)) + offset
	default:
		return 0, fmt.Errorf("invalid whence: %d", whence)
	}
	if newOffset < 0 || newOffset > int64(len(f.fsEntry.Contents)) {
		return 0, fmt.Errorf("invalid new offset: %d", newOffset)
	}
	f.offset = newOffset
	return f.offset, nil
}

// memDir represents an opened memFS directory.
// memDir implements fs.File, fs.ReadDirFile, and io.Seeker.
type memDir struct {
	// name of the opened file, not a full path but a base name.
	name string
	// Seek offset of this opened directory.
	offset int64
	// The memFSEntry this memFileInfo is about.
	fsEntry *memFSEntry
}

// Stat implements fs.File.Stat
func (d *memDir) Stat() (fs.FileInfo, error) {
	return &memFileInfo{name: d.name, fsEntry: d.fsEntry}, nil
}

// Read implements fs.File.Read
func (d *memDir) Read(p []byte) (int, error) {
	return 0, fmt.Errorf("read is not supported by directory")
}

// Close implements fs.File.Close
func (d *memDir) Close() error {
	d.fsEntry = nil
	return nil
}

// Seek implements io.Seeker
func (d *memDir) Seek(offset int64, whence int) (int64, error) {
	var newOffset int64
	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset = d.offset + offset
	case io.SeekEnd:
		newOffset = int64(len(d.fsEntry.Entries)) + offset
	default:
		return 0, fmt.Errorf("invalid whence: %d", whence)
	}
	if newOffset < 0 || newOffset > int64(len(d.fsEntry.Entries)) {
		return 0, fmt.Errorf("invalid new offset: %d", newOffset)
	}
	d.offset = newOffset
	return d.offset, nil
}

// ReadDir implements fs.ReadDirFile.
func (d *memDir) ReadDir(n int) ([]fs.DirEntry, error) {
	// Note that it's ok to return inconsistent list if the directory is modified after previous ReadDir.
	// The result of modifying directory during ReadDir calls is undefined in POSIX, so WASI will be the same.
	entries := make([]fs.DirEntry, 0, len(d.fsEntry.Entries))
	for name, entry := range d.fsEntry.Entries {
		entries = append(entries, &memFileInfo{name: path.Base(name), fsEntry: entry})
	}
	// fs.FeadDirFile.ReadDir requires the result to be sorted by their names.
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	remaining := int64(len(entries)) - d.offset
	if remaining == 0 &&
		n > 0 {
		// return io.EOF only when n > 0, since ReadDir should return empty slice
		// instead of io.EOF when n <= 0. See fs.ReadDir.
		return []fs.DirEntry{}, io.EOF
	}

	if n <= 0 || n > int(remaining) {
		n = int(remaining)
	}
	d.offset += int64(n)

	return entries[d.offset-int64(n) : d.offset], nil
}

// memFileInfo represents a FileInfo of an opened memFile or memDir.
// memFileInfo implements fs.FileInfo and fs.DirEntry
type memFileInfo struct {
	// name of this file, not a full path but a base name.
	name string
	// The memFSEntry this memFileInfo is about.
	fsEntry *memFSEntry
}

// Name implements fs.FileInfo.Name
func (e *memFileInfo) Name() string { return e.name }

// Size implements fs.FileInfo.Size
func (e *memFileInfo) Size() int64 { return int64(len(e.fsEntry.Contents)) }

// Mode implements fs.FileInfo.Mode
func (e *memFileInfo) Mode() fs.FileMode {
	mode := fs.FileMode(0777)
	if e.fsEntry.IsDir() {
		mode |= fs.ModeDir
	}
	return mode
}

// Type implements fs.FileInfo.Type
func (e *memFileInfo) Type() fs.FileMode { return e.Mode().Type() }

// ModTime implements fs.FileInfo.ModTime
func (e *memFileInfo) ModTime() time.Time { return time.Time{} } // return the empty value for now

// IsDir implements fs.FileInfo.IsDir
func (f *memFileInfo) IsDir() bool { return f.Mode().IsDir() }

// Sys implements fs.FileInfo.Sys
func (f *memFileInfo) Sys() interface{} { return nil }

// Info implements fs.DirEntry.Info
func (f *memFileInfo) Info() (fs.FileInfo, error) { return f, nil }

// readerWriterFile implements fs.File and io.Writer.
// If Reader or Writer is nil, Read and Write are no-op just like /dev/null file on unix.
type readerWriterFile struct {
	reader io.Reader
	writer io.Writer
}

// compile-time check that readerWriterFile wraps io.Reader and io.Writer as fs.File.
var _ fs.File = &readerWriterFile{}

// Stat implements fs.File.Stat
func (f *readerWriterFile) Stat() (fs.FileInfo, error) {
	return nil, fmt.Errorf("stat is not supported by readerWriterFile")
}

// Read implements fs.File.Read
func (f *readerWriterFile) Read(p []byte) (int, error) {
	if f.reader == nil {
		return 0, io.EOF
	}
	return f.reader.Read(p)
}

// Write implements fs.File.Write
func (f *readerWriterFile) Write(p []byte) (int, error) {
	if f.writer == nil {
		return len(p), nil
	}
	return f.writer.Write(p)
}

// Close implements fs.File.Close
func (f *readerWriterFile) Close() error {
	f.reader = nil
	f.writer = nil
	return nil
}
