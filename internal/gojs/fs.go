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
	"github.com/tetratelabs/wazero/internal/gojs/util"
	"github.com/tetratelabs/wazero/internal/platform"
	internalsys "github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/sysfs"
	"github.com/tetratelabs/wazero/internal/wasm"
)

var (
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

// jsfs = js.Global().Get("fs") // fs_js.go init
//
// js.fsCall conventions:
// * funcWrapper callback is the last parameter
//   - arg0 is error and up to one result in arg1
func newJsFs(proc *processState) *jsVal {
	return newJsVal(goos.RefJsfs, custom.NameFs).
		addProperties(map[string]interface{}{
			"constants": jsfsConstants, // = jsfs.Get("constants") // init
		}).
		addFunction(custom.NameFsOpen, &jsfsOpen{proc: proc}).
		addFunction(custom.NameFsStat, &jsfsStat{proc: proc}).
		addFunction(custom.NameFsFstat, jsfsFstat{}).
		addFunction(custom.NameFsLstat, &jsfsLstat{proc: proc}).
		addFunction(custom.NameFsClose, jsfsClose{}).
		addFunction(custom.NameFsRead, jsfsRead{}).
		addFunction(custom.NameFsWrite, jsfsWrite{}).
		addFunction(custom.NameFsReaddir, &jsfsReaddir{proc: proc}).
		addFunction(custom.NameFsMkdir, &jsfsMkdir{proc: proc}).
		addFunction(custom.NameFsRmdir, &jsfsRmdir{proc: proc}).
		addFunction(custom.NameFsRename, &jsfsRename{proc: proc}).
		addFunction(custom.NameFsUnlink, &jsfsUnlink{proc: proc}).
		addFunction(custom.NameFsUtimes, &jsfsUtimes{proc: proc}).
		addFunction(custom.NameFsChmod, &jsfsChmod{proc: proc}).
		addFunction(custom.NameFsFchmod, jsfsFchmod{}).
		addFunction(custom.NameFsChown, &jsfsChown{proc: proc}).
		addFunction(custom.NameFsFchown, jsfsFchown{}).
		addFunction(custom.NameFsLchown, &jsfsLchown{proc: proc}).
		addFunction(custom.NameFsTruncate, &jsfsTruncate{proc: proc}).
		addFunction(custom.NameFsFtruncate, jsfsFtruncate{}).
		addFunction(custom.NameFsReadlink, &jsfsReadlink{proc: proc}).
		addFunction(custom.NameFsLink, &jsfsLink{proc: proc}).
		addFunction(custom.NameFsSymlink, &jsfsSymlink{proc: proc}).
		addFunction(custom.NameFsFsync, jsfsFsync{})
}

// jsfsOpen implements implements jsFn for syscall.Open
//
//	jsFD /* Int */, err := fsCall("open", path, flags, perm)
type jsfsOpen struct {
	proc *processState
}

func (o *jsfsOpen) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := util.ResolvePath(o.proc.cwd, args[0].(string))
	flags := toUint64(args[1]) // flags are derived from constants like oWRONLY
	perm := custom.FromJsMode(goos.ValueToUint32(args[2]), o.proc.umask)
	callback := args[3].(funcWrapper)

	fsc := mod.(*wasm.ModuleInstance).Sys.FS()

	fd, errno := fsc.OpenFile(fsc.RootFS(), path, int(flags), perm)

	return callback.invoke(ctx, mod, goos.RefJsfs, maybeError(errno), fd) // note: error first
}

// jsfsStat implements jsFn for syscall.Stat
//
//	jsSt, err := fsCall("stat", path)
type jsfsStat struct {
	proc *processState
}

func (s *jsfsStat) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := util.ResolvePath(s.proc.cwd, args[0].(string))
	callback := args[1].(funcWrapper)

	stat, err := syscallStat(mod, path)
	return callback.invoke(ctx, mod, goos.RefJsfs, err, stat) // note: error first
}

// syscallStat is like syscall.Stat
func syscallStat(mod api.Module, path string) (*jsSt, error) {
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()

	if st, errno := fsc.RootFS().Stat(path); errno != 0 {
		return nil, errno
	} else {
		return newJsSt(st), nil
	}
}

// jsfsLstat implements jsFn for syscall.Lstat
//
//	jsSt, err := fsCall("lstat", path)
type jsfsLstat struct {
	proc *processState
}

func (l *jsfsLstat) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := util.ResolvePath(l.proc.cwd, args[0].(string))
	callback := args[1].(funcWrapper)

	lstat, err := syscallLstat(mod, path)

	return callback.invoke(ctx, mod, goos.RefJsfs, err, lstat) // note: error first
}

// syscallLstat is like syscall.Lstat
func syscallLstat(mod api.Module, path string) (*jsSt, error) {
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()

	if st, errno := fsc.RootFS().Lstat(path); errno != 0 {
		return nil, errno
	} else {
		return newJsSt(st), nil
	}
}

// jsfsFstat implements jsFn for syscall.Open
//
//	stat, err := fsCall("fstat", fd); err == nil && stat.Call("isDirectory").Bool()
type jsfsFstat struct{}

func (jsfsFstat) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()

	fd := goos.ValueToInt32(args[0])
	callback := args[1].(funcWrapper)

	fstat, err := syscallFstat(fsc, fd)
	return callback.invoke(ctx, mod, goos.RefJsfs, err, fstat) // note: error first
}

// syscallFstat is like syscall.Fstat
func syscallFstat(fsc *internalsys.FSContext, fd int32) (*jsSt, error) {
	f, ok := fsc.LookupFile(fd)
	if !ok {
		return nil, syscall.EBADF
	}

	if st, err := f.Stat(); err != nil {
		return nil, platform.UnwrapOSError(err)
	} else {
		return newJsSt(st), nil
	}
}

func newJsSt(st platform.Stat_t) *jsSt {
	ret := &jsSt{}
	ret.isDir = st.Mode.IsDir()
	ret.dev = st.Dev
	ret.ino = st.Ino
	ret.uid = st.Uid
	ret.gid = st.Gid
	ret.mode = custom.ToJsMode(st.Mode)
	ret.nlink = uint32(st.Nlink)
	ret.size = st.Size
	ret.atimeMs = st.Atim / 1e6
	ret.mtimeMs = st.Mtim / 1e6
	ret.ctimeMs = st.Ctim / 1e6
	return ret
}

// jsfsClose implements jsFn for syscall.Close
type jsfsClose struct{}

func (jsfsClose) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()

	fd := goos.ValueToInt32(args[0])
	callback := args[1].(funcWrapper)

	errno := fsc.CloseFile(fd)

	return jsfsInvoke(ctx, mod, callback, errno)
}

// jsfsRead implements jsFn for syscall.Read and syscall.Pread, called by
// src/internal/poll/fd_unix.go poll.Read.
//
//	n, err := fsCall("read", fd, buf, 0, len(b), nil)
type jsfsRead struct{}

func (jsfsRead) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	fd := goos.ValueToInt32(args[0])
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
func syscallRead(mod api.Module, fd int32, offset interface{}, p []byte) (n uint32, err error) {
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()

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
	fd := goos.ValueToInt32(args[0])
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
func syscallWrite(mod api.Module, fd int32, offset interface{}, p []byte) (n uint32, err error) {
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()

	var writer io.Writer
	if f, ok := fsc.LookupFile(fd); !ok {
		err = syscall.EBADF
	} else if offset != nil {
		writer = sysfs.WriterAtOffset(f.File, toInt64(offset))
	} else if writer, ok = f.File.(io.Writer); !ok {
		err = syscall.EBADF
	}

	if err != nil {
		return
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
type jsfsReaddir struct {
	proc *processState
}

func (r *jsfsReaddir) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := util.ResolvePath(r.proc.cwd, args[0].(string))
	callback := args[1].(funcWrapper)

	stat, err := syscallReaddir(ctx, mod, path)
	return callback.invoke(ctx, mod, goos.RefJsfs, err, stat) // note: error first
}

func syscallReaddir(_ context.Context, mod api.Module, name string) (*objectArray, error) {
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()

	// don't allocate a file descriptor
	f, errno := fsc.RootFS().OpenFile(name, os.O_RDONLY, 0)
	if errno != 0 {
		return nil, errno
	}
	defer f.Close() //nolint

	if names, errno := platform.Readdirnames(f, -1); errno != 0 {
		return nil, errno
	} else {
		entries := make([]interface{}, 0, len(names))
		for _, e := range names {
			entries = append(entries, e)
		}
		return &objectArray{entries}, nil
	}
}

// jsfsMkdir implements implements jsFn for fs.Mkdir
//
//	jsFD /* Int */, err := fsCall("mkdir", path, perm)
type jsfsMkdir struct {
	proc *processState
}

func (m *jsfsMkdir) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := util.ResolvePath(m.proc.cwd, args[0].(string))
	perm := custom.FromJsMode(goos.ValueToUint32(args[1]), m.proc.umask)
	callback := args[2].(funcWrapper)

	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	root := fsc.RootFS()

	var fd int32
	var errno syscall.Errno
	// We need at least read access to open the file descriptor
	if perm == 0 {
		perm = 0o0500
	}
	if errno = root.Mkdir(path, perm); errno == 0 {
		fd, errno = fsc.OpenFile(root, path, os.O_RDONLY, 0)
	}

	return callback.invoke(ctx, mod, goos.RefJsfs, maybeError(errno), fd) // note: error first
}

// jsfsRmdir implements jsFn for the following
//
//	_, err := fsCall("rmdir", path) // syscall.Rmdir
type jsfsRmdir struct {
	proc *processState
}

func (r *jsfsRmdir) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := util.ResolvePath(r.proc.cwd, args[0].(string))
	callback := args[1].(funcWrapper)

	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	errno := fsc.RootFS().Rmdir(path)

	return jsfsInvoke(ctx, mod, callback, errno)
}

// jsfsRename implements jsFn for the following
//
//	_, err := fsCall("rename", from, to) // syscall.Rename
type jsfsRename struct {
	proc *processState
}

func (r *jsfsRename) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	cwd := r.proc.cwd
	from := util.ResolvePath(cwd, args[0].(string))
	to := util.ResolvePath(cwd, args[1].(string))
	callback := args[2].(funcWrapper)

	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	errno := fsc.RootFS().Rename(from, to)

	return jsfsInvoke(ctx, mod, callback, errno)
}

// jsfsUnlink implements jsFn for the following
//
//	_, err := fsCall("unlink", path) // syscall.Unlink
type jsfsUnlink struct {
	proc *processState
}

func (u *jsfsUnlink) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := util.ResolvePath(u.proc.cwd, args[0].(string))
	callback := args[1].(funcWrapper)

	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	errno := fsc.RootFS().Unlink(path)

	return jsfsInvoke(ctx, mod, callback, errno)
}

// jsfsUtimes implements jsFn for the following
//
//	_, err := fsCall("utimes", path, atime, mtime) // syscall.Utimens
type jsfsUtimes struct {
	proc *processState
}

func (u *jsfsUtimes) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := util.ResolvePath(u.proc.cwd, args[0].(string))
	atimeSec := toInt64(args[1])
	mtimeSec := toInt64(args[2])
	callback := args[3].(funcWrapper)

	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	times := [2]syscall.Timespec{
		syscall.NsecToTimespec(atimeSec * 1e9), syscall.NsecToTimespec(mtimeSec * 1e9),
	}
	errno := fsc.RootFS().Utimens(path, &times, true)

	return jsfsInvoke(ctx, mod, callback, errno)
}

// jsfsChmod implements jsFn for the following
//
//	_, err := fsCall("chmod", path, mode) // syscall.Chmod
type jsfsChmod struct {
	proc *processState
}

func (c *jsfsChmod) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := util.ResolvePath(c.proc.cwd, args[0].(string))
	mode := custom.FromJsMode(goos.ValueToUint32(args[1]), 0)
	callback := args[2].(funcWrapper)

	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	errno := fsc.RootFS().Chmod(path, mode)

	return jsfsInvoke(ctx, mod, callback, errno)
}

// jsfsFchmod implements jsFn for the following
//
//	_, err := fsCall("fchmod", fd, mode) // syscall.Fchmod
type jsfsFchmod struct{}

func (jsfsFchmod) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	fd := goos.ValueToInt32(args[0])
	mode := custom.FromJsMode(goos.ValueToUint32(args[1]), 0)
	callback := args[2].(funcWrapper)

	// Check to see if the file descriptor is available
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	var errno syscall.Errno
	if f, ok := fsc.LookupFile(fd); !ok {
		errno = syscall.EBADF
	} else if chmodFile, ok := f.File.(chmodFile); !ok {
		errno = syscall.EBADF // possibly a fake file
	} else {
		errno = platform.UnwrapOSError(chmodFile.Chmod(mode))
	}

	return jsfsInvoke(ctx, mod, callback, errno)
}

// jsfsChown implements jsFn for the following
//
//	_, err := fsCall("chown", path, uint32(uid), uint32(gid)) // syscall.Chown
type jsfsChown struct {
	proc *processState
}

func (c *jsfsChown) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := util.ResolvePath(c.proc.cwd, args[0].(string))
	uid := goos.ValueToInt32(args[1])
	gid := goos.ValueToInt32(args[2])
	callback := args[3].(funcWrapper)

	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	errno := fsc.RootFS().Chown(path, int(uid), int(gid))

	return jsfsInvoke(ctx, mod, callback, errno)
}

// jsfsFchown implements jsFn for the following
//
//	_, err := fsCall("fchown", fd, uint32(uid), uint32(gid)) // syscall.Fchown
type jsfsFchown struct{}

func (jsfsFchown) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	fd := goos.ValueToInt32(args[0])
	uid := goos.ValueToUint32(args[1])
	gid := goos.ValueToUint32(args[2])
	callback := args[3].(funcWrapper)

	// Check to see if the file descriptor is available
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	var errno syscall.Errno
	if f, ok := fsc.LookupFile(fd); !ok {
		errno = syscall.EBADF
	} else {
		errno = platform.ChownFile(f.File, int(uid), int(gid))
	}

	return jsfsInvoke(ctx, mod, callback, errno)
}

// jsfsLchown implements jsFn for the following
//
//	_, err := fsCall("lchown", path, uint32(uid), uint32(gid)) // syscall.Lchown
type jsfsLchown struct {
	proc *processState
}

func (l *jsfsLchown) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := util.ResolvePath(l.proc.cwd, args[0].(string))
	uid := goos.ValueToUint32(args[1])
	gid := goos.ValueToUint32(args[2])
	callback := args[3].(funcWrapper)

	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	errno := fsc.RootFS().Lchown(path, int(uid), int(gid))

	return jsfsInvoke(ctx, mod, callback, errno)
}

// jsfsTruncate implements jsFn for the following
//
//	_, err := fsCall("truncate", path, length) // syscall.Truncate
type jsfsTruncate struct {
	proc *processState
}

func (t *jsfsTruncate) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := util.ResolvePath(t.proc.cwd, args[0].(string))
	length := toInt64(args[1])
	callback := args[2].(funcWrapper)

	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	errno := fsc.RootFS().Truncate(path, length)

	return jsfsInvoke(ctx, mod, callback, errno)
}

// jsfsFtruncate implements jsFn for the following
//
//	_, err := fsCall("ftruncate", fd, length) // syscall.Ftruncate
type jsfsFtruncate struct{}

func (jsfsFtruncate) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	fd := goos.ValueToInt32(args[0])
	length := toInt64(args[1])
	callback := args[2].(funcWrapper)

	// Check to see if the file descriptor is available
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	var errno syscall.Errno
	if f, ok := fsc.LookupFile(fd); !ok {
		errno = syscall.EBADF
	} else if truncateFile, ok := f.File.(truncateFile); !ok {
		errno = syscall.EBADF // possibly a fake file
	} else {
		errno = platform.UnwrapOSError(truncateFile.Truncate(length))
	}

	return jsfsInvoke(ctx, mod, callback, errno)
}

// jsfsReadlink implements jsFn for syscall.Readlink
//
//	dst, err := fsCall("readlink", path) // syscall.Readlink
type jsfsReadlink struct {
	proc *processState
}

func (r *jsfsReadlink) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	path := util.ResolvePath(r.proc.cwd, args[0].(string))
	callback := args[1].(funcWrapper)

	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	dst, errno := fsc.RootFS().Readlink(path)

	return callback.invoke(ctx, mod, goos.RefJsfs, maybeError(errno), dst) // note: error first
}

// jsfsLink implements jsFn for the following
//
//	_, err := fsCall("link", path, link) // syscall.Link
type jsfsLink struct {
	proc *processState
}

func (l *jsfsLink) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	cwd := l.proc.cwd
	path := util.ResolvePath(cwd, args[0].(string))
	link := util.ResolvePath(cwd, args[1].(string))
	callback := args[2].(funcWrapper)

	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	errno := fsc.RootFS().Link(path, link)

	return jsfsInvoke(ctx, mod, callback, errno)
}

// jsfsSymlink implements jsFn for the following
//
//	_, err := fsCall("symlink", path, link) // syscall.Symlink
type jsfsSymlink struct {
	proc *processState
}

func (s *jsfsSymlink) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	dst := args[0].(string) // The dst of a symlink must not be resolved, as it should be resolved during readLink.
	link := util.ResolvePath(s.proc.cwd, args[1].(string))
	callback := args[2].(funcWrapper)

	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	errno := fsc.RootFS().Symlink(dst, link)

	return jsfsInvoke(ctx, mod, callback, errno)
}

// jsfsFsync implements jsFn for the following
//
//	_, err := fsCall("fsync", fd) // syscall.Fsync
type jsfsFsync struct{}

func (jsfsFsync) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	fd := goos.ValueToInt32(args[0])
	callback := args[1].(funcWrapper)

	// Check to see if the file descriptor is available
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	var errno syscall.Errno
	if f, ok := fsc.LookupFile(fd); !ok {
		errno = syscall.EBADF
	} else if syncFile, ok := f.File.(syncFile); !ok {
		errno = syscall.EBADF // possibly a fake file
	} else {
		errno = platform.UnwrapOSError(syncFile.Sync())
	}

	return jsfsInvoke(ctx, mod, callback, errno)
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
	return fmt.Sprintf("{isDir=%v,mode=%s,size=%d,mtimeMs=%d}", s.isDir, custom.FromJsMode(s.mode, 0), s.size, s.mtimeMs)
}

// Get implements the same method as documented on goos.GetFunction
func (s *jsSt) Get(propertyKey string) interface{} {
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

func jsfsInvoke(ctx context.Context, mod api.Module, callback funcWrapper, err syscall.Errno) (interface{}, error) {
	return callback.invoke(ctx, mod, goos.RefJsfs, maybeError(err), err == 0) // note: error first
}

func maybeError(errno syscall.Errno) error {
	if errno != 0 {
		return errno
	}
	return nil
}
