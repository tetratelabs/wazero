package gojs

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"syscall"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/gojs/custom"
	"github.com/tetratelabs/wazero/internal/gojs/goos"
	"github.com/tetratelabs/wazero/internal/platform"
	internalsys "github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/sysfs"
	"github.com/tetratelabs/wazero/internal/wasm"
)

var (
	// jsfs = js.Global().Get("fs") // fs_js.go init
	//
	// js.fsCall conventions:
	// * funcWrapper callback is the last parameter
	//   * arg0 is error and up to one result in arg1
	jsfs = newJsVal(goos.RefJsfs, custom.NameFs).
		addProperties(map[string]interface{}{
			"constants": jsfsConstants, // = jsfs.Get("constants") // init
		}).
		addFunction(custom.NameFsOpen, jsfsOpen{}).
		addFunction(custom.NameFsStat, jsfsStat{}).
		addFunction(custom.NameFsFstat, jsfsFstat{}).
		addFunction(custom.NameFsLstat, jsfsLstat{}).
		addFunction(custom.NameFsClose, jsfsClose{}).
		addFunction(custom.NameFsRead, jsfsRead{}).
		addFunction(custom.NameFsWrite, jsfsWrite{}).
		addFunction(custom.NameFsReaddir, jsfsReaddir{}).
		addFunction(custom.NameFsMkdir, jsfsMkdir{}).
		addFunction(custom.NameFsRmdir, jsfsRmdir{}).
		addFunction(custom.NameFsRename, jsfsRename{}).
		addFunction(custom.NameFsUnlink, jsfsUnlink{}).
		addFunction(custom.NameFsUtimes, jsfsUtimes{}).
		addFunction(custom.NameFsChmod, jsfsChmod{}).
		addFunction(custom.NameFsFchmod, jsfsFchmod{}).
		addFunction(custom.NameFsChown, jsfsChown{}).
		addFunction(custom.NameFsFchown, jsfsFchown{}).
		addFunction(custom.NameFsLchown, jsfsLchown{}).
		addFunction(custom.NameFsTruncate, jsfsTruncate{}).
		addFunction(custom.NameFsFtruncate, jsfsFtruncate{}).
		addFunction(custom.NameFsReadlink, jsfsReadlink{}).
		addFunction(custom.NameFsLink, jsfsLink{}).
		addFunction(custom.NameFsSymlink, jsfsSymlink{}).
		addFunction(custom.NameFsFsync, jsfsFsync{})

	// jsfsConstants = jsfs Get("constants") // fs_js.go init
	jsfsConstants = newJsVal(goos.RefJsfsConstants, "constants").
			addProperties(map[string]interface{}{
			"O_WRONLY": oWRONLY,
			"O_RDWR":   oRDWR,
			"O_CREAT":  oCREAT,
			"O_TRUNC":  oTRUNC,
			"O_APPEND": oAPPEND,
			"O_EXCL":   oEXCL,
		})

	// oWRONLY = jsfsConstants Get("O_WRONLY").Int() // fs_js.go init
	oWRONLY = float64(os.O_WRONLY)

	// oRDWR = jsfsConstants Get("O_RDWR").Int() // fs_js.go init
	oRDWR = float64(os.O_RDWR)

	// o CREAT = jsfsConstants Get("O_CREAT").Int() // fs_js.go init
	oCREAT = float64(os.O_CREATE)

	// oTRUNC = jsfsConstants Get("O_TRUNC").Int() // fs_js.go init
	oTRUNC = float64(os.O_TRUNC)

	// oAPPEND = jsfsConstants Get("O_APPEND").Int() // fs_js.go init
	oAPPEND = float64(os.O_APPEND)

	// oEXCL = jsfsConstants Get("O_EXCL").Int() // fs_js.go init
	oEXCL = float64(os.O_EXCL)
)

// The following interfaces are used until we finalize our own FD-scoped file.
type (
	// chmodFile is implemented by os.File in file_posix.go
	chmodFile interface{ Chmod(fs.FileMode) error }
	// syncFile is implemented by os.File in file_posix.go
	syncFile interface{ Sync() error }
	// truncateFile is implemented by os.File in file_posix.go
	truncateFile interface{ Truncate(size int64) error }
)

// jsfsOpen implements implements jsFn for syscall.Open
//
//	jsFD /* Int */, err := fsCall("open", path, flags, perm)
type jsfsOpen struct{}

func (jsfsOpen) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := args[0].(string)
	flags := toUint64(args[1]) // flags are derived from constants like oWRONLY
	perm := goos.ValueToUint32(args[2])
	callback := args[3].(funcWrapper)

	fsc := mod.(*wasm.CallContext).Sys.FS()

	fd, err := fsc.OpenFile(fsc.RootFS(), path, int(flags), fs.FileMode(perm))

	return callback.invoke(ctx, mod, goos.RefJsfs, err, fd) // note: error first
}

// jsfsStat implements jsFn for syscall.Stat
//
//	jsSt, err := fsCall("stat", path)
type jsfsStat struct{}

func (jsfsStat) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := args[0].(string)
	callback := args[1].(funcWrapper)

	stat, err := syscallStat(mod, path)
	return callback.invoke(ctx, mod, goos.RefJsfs, err, stat) // note: error first
}

// syscallStat is like syscall.Stat
func syscallStat(mod api.Module, path string) (*jsSt, error) {
	fsc := mod.(*wasm.CallContext).Sys.FS()

	var stat platform.Stat_t
	if err := fsc.RootFS().Stat(path, &stat); err != nil {
		return nil, err
	}
	return newJsSt(&stat), nil
}

// jsfsLstat implements jsFn for syscall.Lstat
//
//	jsSt, err := fsCall("lstat", path)
type jsfsLstat struct{}

func (jsfsLstat) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := args[0].(string)
	callback := args[1].(funcWrapper)

	lstat, err := syscallLstat(mod, path)

	return callback.invoke(ctx, mod, goos.RefJsfs, err, lstat) // note: error first
}

// syscallLstat is like syscall.Lstat
func syscallLstat(mod api.Module, path string) (*jsSt, error) {
	fsc := mod.(*wasm.CallContext).Sys.FS()

	var stat platform.Stat_t
	if err := fsc.RootFS().Lstat(path, &stat); err != nil {
		return nil, err
	}
	return newJsSt(&stat), nil
}

// jsfsFstat implements jsFn for syscall.Open
//
//	stat, err := fsCall("fstat", fd); err == nil && stat.Call("isDirectory").Bool()
type jsfsFstat struct{}

func (jsfsFstat) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	fsc := mod.(*wasm.CallContext).Sys.FS()

	fd := goos.ValueToUint32(args[0])
	callback := args[1].(funcWrapper)

	fstat, err := syscallFstat(fsc, fd)
	return callback.invoke(ctx, mod, goos.RefJsfs, err, fstat) // note: error first
}

// mode constants from syscall_js.go
const (
	S_IFSOCK = uint32(0o000140000)
	S_IFLNK  = uint32(0o000120000)
	S_IFREG  = uint32(0o000100000)
	S_IFBLK  = uint32(0o000060000)
	S_IFDIR  = uint32(0o000040000)
	S_IFCHR  = uint32(0o000020000)
	S_IFIFO  = uint32(0o000010000)

	S_ISUID = uint32(0o004000)
	S_ISGID = uint32(0o002000)
	S_ISVTX = uint32(0o001000)
)

// syscallFstat is like syscall.Fstat
func syscallFstat(fsc *internalsys.FSContext, fd uint32) (*jsSt, error) {
	f, ok := fsc.LookupFile(fd)
	if !ok {
		return nil, syscall.EBADF
	}

	var st platform.Stat_t
	if err := f.Stat(&st); err != nil {
		return nil, err
	}
	return newJsSt(&st), nil
}

func newJsSt(stat *platform.Stat_t) *jsSt {
	ret := &jsSt{}
	ret.isDir = stat.Mode.IsDir()
	ret.dev = stat.Dev
	ret.ino = stat.Ino
	ret.mode = getJsMode(stat.Mode)
	ret.nlink = uint32(stat.Nlink)
	ret.size = stat.Size
	ret.atimeMs = stat.Atim / 1e6
	ret.mtimeMs = stat.Mtim / 1e6
	ret.ctimeMs = stat.Ctim / 1e6
	return ret
}

// getJsMode is required because the mode property read in `GOOS=js` is
// incompatible with normal go. Particularly the directory flag isn't the same.
func getJsMode(mode fs.FileMode) (jsMode uint32) {
	jsMode = uint32(mode & fs.ModePerm)
	switch mode & fs.ModeType {
	case 0:
		jsMode |= S_IFREG
	case fs.ModeDir:
		jsMode |= S_IFDIR
	case fs.ModeSymlink:
		jsMode |= S_IFLNK
	case fs.ModeNamedPipe:
		jsMode |= S_IFIFO
	case fs.ModeSocket:
		jsMode |= S_IFSOCK
	case fs.ModeDevice:
		jsMode |= S_IFBLK
	case fs.ModeCharDevice:
		jsMode |= S_IFCHR
	case fs.ModeIrregular:
		// unmapped to js
	}

	if mode&fs.ModeSetgid != 0 {
		jsMode |= S_ISGID
	}
	if mode&fs.ModeSetuid != 0 {
		jsMode |= S_ISUID
	}
	if mode&fs.ModeSticky != 0 {
		jsMode |= S_ISVTX
	}
	return
}

// jsfsClose implements jsFn for syscall.Close
type jsfsClose struct{}

func (jsfsClose) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	fsc := mod.(*wasm.CallContext).Sys.FS()

	fd := goos.ValueToUint32(args[0])
	callback := args[1].(funcWrapper)

	err := fsc.CloseFile(fd)

	return jsfsInvoke(ctx, mod, callback, err)
}

// jsfsRead implements jsFn for syscall.Read and syscall.Pread, called by
// src/internal/poll/fd_unix.go poll.Read.
//
//	n, err := fsCall("read", fd, buf, 0, len(b), nil)
type jsfsRead struct{}

func (jsfsRead) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	fd := goos.ValueToUint32(args[0])
	buf, ok := args[1].(*goos.ByteArray)
	if !ok {
		return nil, fmt.Errorf("arg[1] is %v not a []byte", args[1])
	}
	offset := goos.ValueToUint32(args[2])
	byteCount := goos.ValueToUint32(args[3])
	fOffset := args[4] // nil unless Pread
	callback := args[5].(funcWrapper)

	n, err := syscallRead(mod, fd, fOffset, buf.Unwrap()[offset:offset+byteCount])
	return callback.invoke(ctx, mod, goos.RefJsfs, err, n) // note: error first
}

// syscallRead is like syscall.Read
func syscallRead(mod api.Module, fd uint32, offset interface{}, p []byte) (n uint32, err error) {
	fsc := mod.(*wasm.CallContext).Sys.FS()

	f, ok := fsc.LookupFile(fd)
	if !ok {
		err = syscall.EBADF
	}

	var reader io.Reader = f.File

	if offset != nil {
		reader = sysfs.ReaderAtOffset(f.File, toInt64(offset))
	}

	if nRead, e := reader.Read(p); e == nil || e == io.EOF {
		// fs_js.go cannot parse io.EOF so coerce it to nil.
		// See https://github.com/golang/go/issues/43913
		n = uint32(nRead)
	} else {
		err = e
	}
	return
}

// jsfsWrite implements jsFn for syscall.Write and syscall.Pwrite.
//
// Notably, offset is non-nil in Pwrite.
//
//	n, err := fsCall("write", fd, buf, 0, len(b), nil)
type jsfsWrite struct{}

func (jsfsWrite) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	fd := goos.ValueToUint32(args[0])
	buf, ok := args[1].(*goos.ByteArray)
	if !ok {
		return nil, fmt.Errorf("arg[1] is %v not a []byte", args[1])
	}
	offset := goos.ValueToUint32(args[2])
	byteCount := goos.ValueToUint32(args[3])
	fOffset := args[4] // nil unless Pwrite
	callback := args[5].(funcWrapper)

	if byteCount > 0 { // empty is possible on EOF
		n, err := syscallWrite(mod, fd, fOffset, buf.Unwrap()[offset:offset+byteCount])
		return callback.invoke(ctx, mod, goos.RefJsfs, err, n) // note: error first
	}
	return callback.invoke(ctx, mod, goos.RefJsfs, nil, goos.RefValueZero)
}

// syscallWrite is like syscall.Write
func syscallWrite(mod api.Module, fd uint32, offset interface{}, p []byte) (n uint32, err error) {
	fsc := mod.(*wasm.CallContext).Sys.FS()

	var writer io.Writer
	if f, ok := fsc.LookupFile(fd); !ok {
		err = syscall.EBADF
	} else if offset != nil {
		writer = sysfs.WriterAtOffset(f.File, toInt64(offset))
	} else {
		writer = f.File.(io.Writer)
	}

	if nWritten, e := writer.Write(p); e == nil || e == io.EOF {
		// fs_js.go cannot parse io.EOF so coerce it to nil.
		// See https://github.com/golang/go/issues/43913
		n = uint32(nWritten)
	} else {
		err = e
	}
	return
}

// jsfsReaddir implements jsFn for syscall.Open
//
//	dir, err := fsCall("readdir", path)
//		dir.Length(), dir.Index(i).String()
type jsfsReaddir struct{}

func (jsfsReaddir) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := args[0].(string)
	callback := args[1].(funcWrapper)

	stat, err := syscallReaddir(ctx, mod, path)
	return callback.invoke(ctx, mod, goos.RefJsfs, err, stat) // note: error first
}

func syscallReaddir(_ context.Context, mod api.Module, name string) (*objectArray, error) {
	fsc := mod.(*wasm.CallContext).Sys.FS()

	// don't allocate a file descriptor
	f, err := fsc.RootFS().OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint

	if names, err := platform.Readdirnames(f, -1); err != nil {
		return nil, err
	} else {
		entries := make([]interface{}, 0, len(names))
		for _, e := range names {
			entries = append(entries, e)
		}
		return &objectArray{entries}, nil
	}
}

// returnZero implements jsFn
type returnZero struct{}

func (returnZero) invoke(context.Context, api.Module, ...interface{}) (interface{}, error) {
	return goos.RefValueZero, nil
}

// returnSliceOfZero implements jsFn
type returnSliceOfZero struct{}

func (returnSliceOfZero) invoke(context.Context, api.Module, ...interface{}) (interface{}, error) {
	return &objectArray{slice: []interface{}{goos.RefValueZero}}, nil
}

// returnArg0 implements jsFn
type returnArg0 struct{}

func (returnArg0) invoke(_ context.Context, _ api.Module, args ...interface{}) (interface{}, error) {
	return args[0], nil
}

// processCwd implements jsFn for fs.Open syscall.Getcwd in fs_js.go
type processCwd struct{}

func (processCwd) invoke(ctx context.Context, _ api.Module, _ ...interface{}) (interface{}, error) {
	return getState(ctx).cwd, nil
}

// processChdir implements jsFn for fs.Open syscall.Chdir in fs_js.go
type processChdir struct{}

func (processChdir) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := args[0].(string)

	if s, err := syscallStat(mod, path); err != nil {
		return nil, err
	} else if !s.isDir {
		return nil, syscall.ENOTDIR
	} else {
		getState(ctx).cwd = path
		return nil, nil
	}
}

// jsfsMkdir implements implements jsFn for fs.Mkdir
//
//	jsFD /* Int */, err := fsCall("mkdir", path, perm)
type jsfsMkdir struct{}

func (jsfsMkdir) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := args[0].(string)
	perm := goos.ValueToUint32(args[1])
	callback := args[2].(funcWrapper)

	fsc := mod.(*wasm.CallContext).Sys.FS()
	root := fsc.RootFS()

	var fd uint32
	var err error
	if err = root.Mkdir(path, fs.FileMode(perm)); err == nil {
		fd, err = fsc.OpenFile(root, path, os.O_RDONLY, 0)
	}

	return callback.invoke(ctx, mod, goos.RefJsfs, err, fd) // note: error first
}

// jsfsRmdir implements jsFn for the following
//
//	_, err := fsCall("rmdir", path) // syscall.Rmdir
type jsfsRmdir struct{}

func (jsfsRmdir) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := args[0].(string)
	callback := args[1].(funcWrapper)

	fsc := mod.(*wasm.CallContext).Sys.FS()
	err := fsc.RootFS().Rmdir(path)

	return jsfsInvoke(ctx, mod, callback, err)
}

// jsfsRename implements jsFn for the following
//
//	_, err := fsCall("rename", from, to) // syscall.Rename
type jsfsRename struct{}

func (jsfsRename) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	from := args[0].(string)
	to := args[1].(string)
	callback := args[2].(funcWrapper)

	fsc := mod.(*wasm.CallContext).Sys.FS()
	err := fsc.RootFS().Rename(from, to)

	return jsfsInvoke(ctx, mod, callback, err)
}

// jsfsUnlink implements jsFn for the following
//
//	_, err := fsCall("unlink", path) // syscall.Unlink
type jsfsUnlink struct{}

func (jsfsUnlink) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := args[0].(string)
	callback := args[1].(funcWrapper)

	fsc := mod.(*wasm.CallContext).Sys.FS()
	err := fsc.RootFS().Unlink(path)

	return jsfsInvoke(ctx, mod, callback, err)
}

// jsfsUtimes implements jsFn for the following
//
//	_, err := fsCall("utimes", path, atime, mtime) // syscall.UtimesNano
type jsfsUtimes struct{}

func (jsfsUtimes) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := args[0].(string)
	atimeSec := toInt64(args[1])
	mtimeSec := toInt64(args[2])
	callback := args[3].(funcWrapper)

	fsc := mod.(*wasm.CallContext).Sys.FS()
	err := fsc.RootFS().Utimes(path, atimeSec*1e9, mtimeSec*1e9)

	return jsfsInvoke(ctx, mod, callback, err)
}

// jsfsChmod implements jsFn for the following
//
//	_, err := fsCall("chmod", path, mode) // syscall.Chmod
type jsfsChmod struct{}

func (jsfsChmod) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := args[0].(string)
	mode := goos.ValueToUint32(args[1])
	callback := args[2].(funcWrapper)

	fsc := mod.(*wasm.CallContext).Sys.FS()
	err := fsc.RootFS().Chmod(path, fs.FileMode(mode))

	return jsfsInvoke(ctx, mod, callback, err)
}

// jsfsFchmod implements jsFn for the following
//
//	_, err := fsCall("fchmod", fd, mode) // syscall.Fchmod
type jsfsFchmod struct{}

func (jsfsFchmod) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	fd := goos.ValueToUint32(args[0])
	mode := goos.ValueToUint32(args[1])
	callback := args[2].(funcWrapper)

	// Check to see if the file descriptor is available
	fsc := mod.(*wasm.CallContext).Sys.FS()
	var err error
	if f, ok := fsc.LookupFile(fd); !ok {
		err = syscall.EBADF
	} else if chmodFile, ok := f.File.(chmodFile); !ok {
		err = syscall.EBADF // possibly a fake file
	} else {
		err = chmodFile.Chmod(fs.FileMode(mode))
	}

	return jsfsInvoke(ctx, mod, callback, err)
}

// jsfsChown implements jsFn for the following
//
//	_, err := fsCall("chown", path, uint32(uid), uint32(gid)) // syscall.Chown
type jsfsChown struct{}

func (jsfsChown) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := args[0].(string)
	uid := goos.ValueToUint32(args[1])
	gid := goos.ValueToUint32(args[2])
	callback := args[3].(funcWrapper)

	_, _, _ = path, uid, gid // TODO
	var err error = syscall.ENOSYS

	return jsfsInvoke(ctx, mod, callback, err)
}

// jsfsFchown implements jsFn for the following
//
//	_, err := fsCall("fchown", fd, uint32(uid), uint32(gid)) // syscall.Fchown
type jsfsFchown struct{}

func (jsfsFchown) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	fd := goos.ValueToUint32(args[0])
	uid := goos.ValueToUint32(args[1])
	gid := goos.ValueToUint32(args[2])
	callback := args[3].(funcWrapper)

	_, _, _ = fd, uid, gid // TODO
	var err error = syscall.ENOSYS

	return jsfsInvoke(ctx, mod, callback, err)
}

// jsfsLchown implements jsFn for the following
//
//	_, err := fsCall("lchown", path, uint32(uid), uint32(gid)) // syscall.Lchown
type jsfsLchown struct{}

func (jsfsLchown) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := args[0].(string)
	uid := goos.ValueToUint32(args[1])
	gid := goos.ValueToUint32(args[2])
	callback := args[3].(funcWrapper)

	_, _, _ = path, uid, gid // TODO
	var err error = syscall.ENOSYS

	return jsfsInvoke(ctx, mod, callback, err)
}

// jsfsTruncate implements jsFn for the following
//
//	_, err := fsCall("truncate", path, length) // syscall.Truncate
type jsfsTruncate struct{}

func (jsfsTruncate) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := args[0].(string)
	length := toInt64(args[1])
	callback := args[2].(funcWrapper)

	fsc := mod.(*wasm.CallContext).Sys.FS()
	err := fsc.RootFS().Truncate(path, length)

	return jsfsInvoke(ctx, mod, callback, err)
}

// jsfsFtruncate implements jsFn for the following
//
//	_, err := fsCall("ftruncate", fd, length) // syscall.Ftruncate
type jsfsFtruncate struct{}

func (jsfsFtruncate) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	fd := goos.ValueToUint32(args[0])
	length := toInt64(args[1])
	callback := args[2].(funcWrapper)

	// Check to see if the file descriptor is available
	fsc := mod.(*wasm.CallContext).Sys.FS()
	var err error
	if f, ok := fsc.LookupFile(fd); !ok {
		err = syscall.EBADF
	} else if truncateFile, ok := f.File.(truncateFile); !ok {
		err = syscall.EBADF // possibly a fake file
	} else {
		err = truncateFile.Truncate(length)
	}

	return jsfsInvoke(ctx, mod, callback, err)
}

// jsfsReadlink implements jsFn for syscall.Readlink
//
//	dst, err := fsCall("readlink", path) // syscall.Readlink
type jsfsReadlink struct{}

func (jsfsReadlink) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := args[0].(string)
	callback := args[1].(funcWrapper)

	_ = path // TODO
	var dst string
	var err error = syscall.ENOSYS

	return callback.invoke(ctx, mod, goos.RefJsfs, err, dst) // note: error first
}

// jsfsLink implements jsFn for the following
//
//	_, err := fsCall("link", path, link) // syscall.Link
type jsfsLink struct{}

func (jsfsLink) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := args[0].(string)
	link := args[1].(string)
	callback := args[2].(funcWrapper)

	_, _ = path, link // TODO
	var err error = syscall.ENOSYS

	return jsfsInvoke(ctx, mod, callback, err)
}

// jsfsSymlink implements jsFn for the following
//
//	_, err := fsCall("symlink", path, link) // syscall.Symlink
type jsfsSymlink struct{}

func (jsfsSymlink) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := args[0].(string)
	link := args[1].(string)
	callback := args[2].(funcWrapper)

	fsc := mod.(*wasm.CallContext).Sys.FS()
	err := fsc.RootFS().Symlink(path, link)

	return jsfsInvoke(ctx, mod, callback, err)
}

// jsfsFsync implements jsFn for the following
//
//	_, err := fsCall("fsync", fd) // syscall.Fsync
type jsfsFsync struct{}

func (jsfsFsync) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	fd := goos.ValueToUint32(args[0])
	callback := args[1].(funcWrapper)

	// Check to see if the file descriptor is available
	fsc := mod.(*wasm.CallContext).Sys.FS()
	var err error
	if f, ok := fsc.LookupFile(fd); !ok {
		err = syscall.EBADF
	} else if syncFile, ok := f.File.(syncFile); !ok {
		err = syscall.EBADF // possibly a fake file
	} else {
		err = syncFile.Sync()
	}

	return jsfsInvoke(ctx, mod, callback, err)
}

// jsSt is pre-parsed from fs_js.go setStat to avoid thrashing
type jsSt struct {
	isDir   bool
	dev     uint64
	ino     uint64
	mode    uint32
	nlink   uint32
	uid     uint32
	gid     uint32
	rdev    int64
	size    int64
	blksize int32
	blocks  int32
	atimeMs int64
	mtimeMs int64
	ctimeMs int64
}

// String implements fmt.Stringer
func (s *jsSt) String() string {
	return fmt.Sprintf("{isDir=%v,mode=%s,size=%d,mtimeMs=%d}", s.isDir, fs.FileMode(s.mode), s.size, s.mtimeMs)
}

// Get implements the same method as documented on goos.GetFunction
func (s *jsSt) Get(_ context.Context, propertyKey string) interface{} {
	switch propertyKey {
	case "dev":
		return s.dev
	case "ino":
		return s.ino
	case "mode":
		return s.mode
	case "nlink":
		return s.nlink
	case "uid":
		return s.uid
	case "gid":
		return s.gid
	case "rdev":
		return s.rdev
	case "size":
		return s.size
	case "blksize":
		return s.blksize
	case "blocks":
		return s.blocks
	case "atimeMs":
		return s.atimeMs
	case "mtimeMs":
		return s.mtimeMs
	case "ctimeMs":
		return s.ctimeMs
	}
	panic(fmt.Sprintf("TODO: stat.%s", propertyKey))
}

// call implements jsCall.call
func (s *jsSt) call(_ context.Context, _ api.Module, _ goos.Ref, method string, _ ...interface{}) (interface{}, error) {
	if method == "isDirectory" {
		return s.isDir, nil
	}
	panic(fmt.Sprintf("TODO: stat.%s", method))
}

func jsfsInvoke(ctx context.Context, mod api.Module, callback funcWrapper, err error) (interface{}, error) {
	return callback.invoke(ctx, mod, goos.RefJsfs, err, err == nil) // note: error first
}
