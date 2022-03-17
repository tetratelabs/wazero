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

// DirFS is a file system that implements fs.FS and wasi.OpenFileFS.
// It is similar to os.DirFS, but supports file creation by OpenFile.
// See os.DirFS.
type DirFS string

// OpenFile implements wasi.OpenFileFS.OpenFile.
func (dir DirFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	if !fs.ValidPath(name) || runtime.GOOS == "windows" && strings.ContainsAny(name, `\:`) {
		return nil, &os.PathError{Op: "open", Path: name, Err: os.ErrInvalid}
	}
	return os.OpenFile(string(dir)+"/"+name, flag, perm)
}

// Open implements fs.FS.Open.
func (dir DirFS) Open(path string) (fs.File, error) {
	return dir.OpenFile(path, os.O_RDONLY, 0)
}

// MemFS is an in-memory file system that implements fs.FS and wasi.OpenFileFS.
type MemFS map[string]*memFSEntry

// OpenFile implements wasi.OpenFileFS.OpenFile.
func (m MemFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &os.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

	dir := &memFSEntry{Entries: m}

	// Path "." indicates m itself as a directory.
	if name == "." {
		return newMemDir(dir, "."), nil
	}

	// Traverse directories.
	files := strings.Split(name, "/")
	for _, file := range files[:len(files)-1] {
		var ok bool
		dir, ok = dir.Entries[file]
		if !ok || !dir.IsDir() {
			return nil, &os.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
	}

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
		return newMemDir(entry, baseName), nil
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
	switch whence {
	case io.SeekStart:
		f.offset = offset
	case io.SeekCurrent:
		f.offset += offset
	case io.SeekEnd:
		f.offset = int64(len(f.fsEntry.Contents)) + offset
	default:
		return 0, fmt.Errorf("invalid whence: %d", whence)
	}
	return f.offset, nil
}

// memDir represents an opened memFS directory.
// memDir implements fs.File, fs.ReadDirFile, and io.Seeker.
type memDir struct {
	// name of the opened file, not a full path but a base name.
	name string
	// Seek offset of this opened directory.
	offset int64
	// Sorted file list of the dirEntries for ReadDir and Seek.
	entries []fs.DirEntry
	// The memFSEntry this memFileInfo is about.
	fsEntry *memFSEntry
}

func newMemDir(dir *memFSEntry, baseName string) *memDir {
	// Cache the file list of the directory for ReadDir to return the result in the consistent order.
	// Note that it's ok to return stale file list if the directory is modified after Open.
	// The result of ReadDir is undefined in POSIX in that situation, so WASI will be the same.
	entries := make([]fs.DirEntry, 0, len(dir.Entries))
	for name, entry := range dir.Entries {
		entries = append(entries, &memFileInfo{name: path.Base(name), fsEntry: entry})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	return &memDir{
		name:    baseName,
		entries: entries,
		fsEntry: dir,
	}
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
	return nil
}

// Seek implements io.Seeker
func (d *memDir) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		d.offset = offset
	case io.SeekCurrent:
		d.offset += offset
	case io.SeekEnd:
		d.offset = int64(len(d.entries)) + offset
	default:
		return 0, fmt.Errorf("invalid whence: %d", whence)
	}
	return d.offset, nil
}

// ReadDir implements fs.ReadDirFile.
func (d *memDir) ReadDir(n int) ([]fs.DirEntry, error) {
	remaining := int64(len(d.entries)) - d.offset
	if remaining == 0 &&
		n > 0 {
		// return io.EOF only when n > 0, since ReadDir should return empty slice
		// instead of io.EOF when n <= 0. See fs.ReadDir.
		return []fs.DirEntry{}, io.EOF
	}

	if n <= 0 || n > int(remaining) {
		n = int(remaining)
	}
	entries := d.entries[d.offset : d.offset+int64(n)]
	d.offset += int64(n)

	return entries, nil
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

// readerWriterFile wraps io.Reader and io.Writer as fs.File.
// readerWriterFile implements fs.File and io.Writer.
// If Reader or Writer is nil, Read and Write are no-op just like /dev/null file on unix.
type readerWriterFile struct {
	Reader io.Reader
	Writer io.Writer
}

// Stat implements fs.File.Stat
func (f *readerWriterFile) Stat() (fs.FileInfo, error) {
	return nil, fmt.Errorf("stat is not supported by readerWriterFile")
}

// Read implements fs.File.Read
func (f *readerWriterFile) Read(p []byte) (int, error) {
	if f.Reader == nil {
		return 0, io.EOF
	}
	return f.Reader.Read(p)
}

// Write implements fs.File.Write
func (f *readerWriterFile) Write(p []byte) (int, error) {
	if f.Writer == nil {
		return len(p), nil
	}
	return f.Writer.Write(p)
}

// Close implements fs.File.Close
func (f *readerWriterFile) Close() error {
	return nil
}
