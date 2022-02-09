package wasi

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"math/rand"
	"os"
	"reflect"
	"time"
	"unicode/utf8"

	"github.com/tetratelabs/wazero/wasm"
)

// API is a documentation interface for WASI exported functions in version "wasi_snapshot_preview1"
//
// Note: In WebAssembly 1.0 (MVP), there may be up to one Memory per store, which means the precise memory is always
// wasm.Store Memories index zero: `store.Memories[0].Buffer`
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md
// See https://www.w3.org/TR/wasm-core-1/#memory-instances%E2%91%A0.
type API interface {
	// ArgsSizesGet is a WASI function that reads command-line argument data (Args) sizes.
	//
	// There are two parameters and an Errno result. Both parameters are offsets in the wasm.MemoryInstance Buffer to
	// write the corresponding sizes in uint32 little-endian encoding.
	//
	// * argc - is the offset to write the argument count to the wasm.MemoryInstance Buffer
	// * argvBufSize - is the offset to write the null terminated argument length to the wasm.MemoryInstance Buffer
	//
	// In WebAssembly 1.0 (MVP) Text format, this signature is:
	//	(import "wasi_snapshot_preview1" "args_sizes_get"
	//		(func ($wasi_args_sizes_get (param $argc i32) (param $argv_buf_size i32) (result i32))
	//
	// For example, if Args are []string{"a","bc"} and
	//   ArgsSizesGet parameters argc=1 and argvBufSize=6, we expect `store.Memories[0].Buffer` to contain:
	//
	//                    uint32         uint32
	//                  +--------+     +--------+
	//                  |        |     |        |
	//        []byte{?, 2, 0, 0, 0, ?, 5, 0, 0, 0, ?}
	//           argc --^              ^
	//         2 args --+              |
	//                   argvBufSize --|
	//   len([]byte{'a',0,'b',c',0}) --+
	//
	// See ArgsGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#args_sizes_get
	// See https://en.wikipedia.org/wiki/Null-terminated_string
	ArgsSizesGet(ctx *wasm.HostFunctionCallContext, argc, argvBufSize uint32) Errno

	// ArgsGet is a WASI function that reads command-line argument data (Args).
	//
	// There are two parameters and an Errno result. Both parameters are offsets in the wasm.MemoryInstance Buffer to
	// write offsets. These are encoded uint32 little-endian.
	//
	// * argv - is the offset to begin writing argument offsets to the wasm.MemoryInstance
	//   * ArgsSizesGet argc * 4 bytes are written to this offset
	// * argvBuf - is the offset to write the null terminated arguments to the wasm.MemoryInstance
	//   * ArgsSizesGet argv_buf_size are written to this offset
	//
	// In WebAssembly 1.0 (MVP) Text format, this signature is:
	//	(import "wasi_snapshot_preview1" "args_get"
	//		(func ($wasi_args_get (param $argv i32) (param $argv_buf i32) (result i32))
	//
	// For example, if ArgsSizesGet wrote argc=2 and argvBufSize=5 for arguments: "a" and "bc"
	//   and ArgsGet parameters argv=7 and argvBuf=1, we expect `store.Memories[0].Buffer` to contain:
	//
	//               argvBufSize                argc * 4
	//            +----------------+     +--------------------+
	//            |                |     |                    |
	// []byte{?, 'a', 0, 'b', 'c', 0, ?, 1, 0, 0, 0, 3, 0, 0, 0, ?}
	//  argvBuf --^                      ^           ^
	//                            argv --|           |
	//          offset that begins "a" --+           |
	//                     offset that begins "bc" --+
	//
	// See ArgsSizesGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#args_get
	// See https://en.wikipedia.org/wiki/Null-terminated_string
	ArgsGet(ctx *wasm.HostFunctionCallContext, argv, argvBuf uint32) Errno
}

const (
	wasiUnstableName         = "wasi_unstable"
	wasiSnapshotPreview1Name = "wasi_snapshot_preview1"
)

type api struct {
	args  *nullTerminatedStrings
	stdin io.Reader
	stdout,
	stderr io.Writer
	opened         map[uint32]fileEntry
	getTimeNanosFn func() uint64
}

func (w *api) register(store *wasm.Store) (err error) {
	for _, wasiName := range []string{
		wasiUnstableName,
		wasiSnapshotPreview1Name,
	} {
		err = store.AddHostFunction(wasiName, "proc_exit", reflect.ValueOf(proc_exit))
		if err != nil {
			return err
		}
		err = store.AddHostFunction(wasiName, "fd_write", reflect.ValueOf(w.fd_write))
		if err != nil {
			return err
		}
		err = store.AddHostFunction(wasiName, "environ_sizes_get", reflect.ValueOf(environ_sizes_get))
		if err != nil {
			return err
		}
		err = store.AddHostFunction(wasiName, "environ_get", reflect.ValueOf(environ_get))
		if err != nil {
			return err
		}
		err = store.AddHostFunction(wasiName, "fd_prestat_get", reflect.ValueOf(w.fd_prestat_get))
		if err != nil {
			return err
		}
		err = store.AddHostFunction(wasiName, "fd_prestat_dir_name", reflect.ValueOf(w.fd_prestat_dir_name))
		if err != nil {
			return err
		}
		err = store.AddHostFunction(wasiName, "fd_fdstat_get", reflect.ValueOf(w.fd_fdstat_get))
		if err != nil {
			return err
		}
		err = store.AddHostFunction(wasiName, "fd_close", reflect.ValueOf(w.fd_close))
		if err != nil {
			return err
		}
		err = store.AddHostFunction(wasiName, "fd_read", reflect.ValueOf(w.fd_read))
		if err != nil {
			return err
		}
		err = store.AddHostFunction(wasiName, "fd_seek", reflect.ValueOf(w.fd_seek))
		if err != nil {
			return err
		}
		err = store.AddHostFunction(wasiName, "path_open", reflect.ValueOf(w.path_open))
		if err != nil {
			return err
		}
		err = store.AddHostFunction(wasiName, "args_get", reflect.ValueOf(w.ArgsGet))
		if err != nil {
			return err
		}
		err = store.AddHostFunction(wasiName, "args_sizes_get", reflect.ValueOf(w.ArgsSizesGet))
		if err != nil {
			return err
		}
		err = store.AddHostFunction(wasiName, "clock_time_get", reflect.ValueOf(w.clock_time_get))
		if err != nil {
			return err
		}
	}
	return nil
}

type fileEntry struct {
	path    string
	fileSys FS
	file    File
}

type Option func(*api)

func Stdin(reader io.Reader) Option {
	return func(w *api) {
		w.stdin = reader
	}
}

func Stdout(writer io.Writer) Option {
	return func(w *api) {
		w.stdout = writer
	}
}

func Stderr(writer io.Writer) Option {
	return func(w *api) {
		w.stderr = writer
	}
}

// nullTerminatedStrings holds null-terminated strings. It ensures that
// its length and total buffer size don't exceed the max of uint32.
// nullTerminatedStrings are convenience struct for args_get and environ_get. (environ_get is not implemented yet)
//
// A Null-terminated string is a byte string with a NULL suffix ("\x00").
// See https://en.wikipedia.org/wiki/Null-terminated_string
type nullTerminatedStrings struct {
	// nullTerminatedValues are null-terminated values with a NULL suffix.
	nullTerminatedValues [][]byte
	totalBufSize         uint32
}

// newNullTerminatedStrings creates a nullTerminatedStrings from the given string slice. It returns an error
// if the length or the total buffer size of the result WASIStringArray exceeds the maxBufSize
func newNullTerminatedStrings(maxBufSize uint32, args ...string) (*nullTerminatedStrings, error) {
	if len(args) == 0 {
		return &nullTerminatedStrings{nullTerminatedValues: [][]byte{}}, nil
	}
	var strings [][]byte // don't pre-allocate as this function is size bound
	totalBufSize := uint32(0)
	for i, arg := range args {
		if !utf8.ValidString(arg) {
			return nil, fmt.Errorf("arg[%d] is not a valid UTF-8 string", i)
		}
		argLen := uint64(len(arg)) + 1 // + 1 for "\x00"; uint64 in case this one arg is huge
		nextSize := uint64(totalBufSize) + argLen
		if nextSize > uint64(maxBufSize) { //
			return nil, fmt.Errorf("arg[%d] will exceed max buffer size %d", i, maxBufSize)
		}
		totalBufSize = uint32(nextSize)
		strings = append(strings, append([]byte(arg), 0))
	}
	return &nullTerminatedStrings{nullTerminatedValues: strings, totalBufSize: totalBufSize}, nil
}

// Args returns an option to give a command-line arguments to the API or errs if the inputs are too large.
//
// Note: The only reason to set this is to control what's written by API.ArgsSizesGet and API.ArgsGet
// Note: While similar in structure to os.Args, this controls what's visible in Wasm (ex the WASI function "_start").
func Args(args ...string) (Option, error) {
	wasiStrings, err := newNullTerminatedStrings(math.MaxUint32, args...) // TODO: this is crazy high even if spec allows it
	if err != nil {
		return nil, err
	}
	return func(w *api) {
		w.args = wasiStrings
	}, nil
}

func Preopen(dir string, fileSys FS) Option {
	return func(w *api) {
		w.opened[uint32(len(w.opened))+3] = fileEntry{
			path:    dir,
			fileSys: fileSys,
		}
	}
}

// RegisterAPI adds each function API to the wasm.Store via AddHostFunction.
func RegisterAPI(store *wasm.Store, opts ...Option) error {
	_, err := registerAPI(store, opts...)
	return err
}

// TODO: we can't export a return with API until we figure out how to give users a wasm.HostFunctionCallContext
func registerAPI(store *wasm.Store, opts ...Option) (API, error) {
	ret := newAPI(opts...)
	err := ret.register(store)
	return ret, err
}

func newAPI(opts ...Option) *api {
	ret := &api{
		args:   &nullTerminatedStrings{},
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
		opened: map[uint32]fileEntry{},
		getTimeNanosFn: func() uint64 {
			return uint64(time.Now().UnixNano())
		},
	}

	// apply functional options
	for _, f := range opts {
		f(ret)
	}
	return ret
}

func (w *api) randUnusedFD() uint32 {
	fd := uint32(rand.Int31())
	for {
		if _, ok := w.opened[fd]; !ok {
			return fd
		}
		fd = (fd + 1) % (1 << 31)
	}
}

func (w *api) fd_prestat_get(ctx *wasm.HostFunctionCallContext, fd uint32, bufPtr uint32) (err Errno) {
	if _, ok := w.opened[fd]; !ok {
		return EBADF
	}
	return ESUCCESS
}

func (w *api) fd_prestat_dir_name(ctx *wasm.HostFunctionCallContext, fd uint32, pathPtr uint32, pathLen uint32) (err Errno) {
	f, ok := w.opened[fd]
	if !ok {
		return EINVAL
	}

	if uint32(len(f.path)) < pathLen {
		return ENAMETOOLONG
	}

	copy(ctx.Memory.Buffer[pathPtr:], f.path)
	return ESUCCESS
}

func (w *api) fd_fdstat_get(ctx *wasm.HostFunctionCallContext, fd uint32, bufPtr uint32) (err Errno) {
	if _, ok := w.opened[fd]; !ok {
		return EBADF
	}
	binary.LittleEndian.PutUint64(ctx.Memory.Buffer[bufPtr+16:], R_FD_READ|R_FD_WRITE)
	return ESUCCESS
}

func (w *api) path_open(ctx *wasm.HostFunctionCallContext, fd, dirFlags, pathPtr, pathLen, oFlags uint32,
	fsRightsBase, fsRightsInheriting uint64,
	fdFlags, fdPtr uint32) (errno Errno) {
	dir, ok := w.opened[fd]
	if !ok || dir.fileSys == nil {
		return EINVAL
	}

	path := string(ctx.Memory.Buffer[pathPtr : pathPtr+pathLen])
	f, err := dir.fileSys.OpenWASI(dirFlags, path, oFlags, fsRightsBase, fsRightsInheriting, fdFlags)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return ENOENT
		default:
			return EINVAL
		}
	}

	newFD := w.randUnusedFD()

	w.opened[newFD] = fileEntry{
		file: f,
	}

	binary.LittleEndian.PutUint32(ctx.Memory.Buffer[fdPtr:], newFD)
	return ESUCCESS
}

func (w *api) fd_seek(ctx *wasm.HostFunctionCallContext, fd uint32, offset uint64, whence uint32, nwrittenPtr uint32) (err Errno) {
	// not implemented yet
	return ENOSYS
}

func (w *api) fd_write(ctx *wasm.HostFunctionCallContext, fd uint32, iovsPtr uint32, iovsLen uint32, nwrittenPtr uint32) (err Errno) {
	var writer io.Writer

	switch fd {
	case 1:
		writer = w.stdout
	case 2:
		writer = w.stderr
	default:
		f, ok := w.opened[fd]
		if !ok || f.file == nil {
			return EBADF
		}
		writer = f.file
	}

	var nwritten uint32
	for i := uint32(0); i < iovsLen; i++ {
		iovPtr := iovsPtr + i*8
		offset := binary.LittleEndian.Uint32(ctx.Memory.Buffer[iovPtr:])
		l := binary.LittleEndian.Uint32(ctx.Memory.Buffer[iovPtr+4:])
		n, err := writer.Write(ctx.Memory.Buffer[offset : offset+l])
		if err != nil {
			panic(err)
		}
		nwritten += uint32(n)
	}
	binary.LittleEndian.PutUint32(ctx.Memory.Buffer[nwrittenPtr:], nwritten)
	return ESUCCESS
}

func (w *api) fd_read(ctx *wasm.HostFunctionCallContext, fd uint32, iovsPtr uint32, iovsLen uint32, nreadPtr uint32) (err Errno) {
	var reader io.Reader

	switch fd {
	case 0:
		reader = w.stdin
	default:
		f, ok := w.opened[fd]
		if !ok || f.file == nil {
			return EBADF
		}
		reader = f.file
	}

	var nread uint32
	for i := uint32(0); i < iovsLen; i++ {
		iovPtr := iovsPtr + i*8
		offset := binary.LittleEndian.Uint32(ctx.Memory.Buffer[iovPtr:])
		l := binary.LittleEndian.Uint32(ctx.Memory.Buffer[iovPtr+4:])
		n, err := reader.Read(ctx.Memory.Buffer[offset : offset+l])
		nread += uint32(n)
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return EIO
		}
	}
	binary.LittleEndian.PutUint32(ctx.Memory.Buffer[nreadPtr:], nread)
	return ESUCCESS
}

func (w *api) fd_close(ctx *wasm.HostFunctionCallContext, fd uint32) (err Errno) {
	f, ok := w.opened[fd]
	if !ok {
		return EBADF
	}

	if f.file != nil {
		f.file.Close()
	}

	delete(w.opened, fd)

	return ESUCCESS
}

// ArgsSizesGet implements API.ArgsSizesGet
func (w *api) ArgsSizesGet(ctx *wasm.HostFunctionCallContext, argc, argvBufSize uint32) Errno {
	if !ctx.Memory.PutUint32(argc, uint32(len(w.args.nullTerminatedValues))) {
		return EINVAL
	}
	if !ctx.Memory.PutUint32(argvBufSize, w.args.totalBufSize) {
		return EINVAL
	}

	return ESUCCESS
}

// ArgsGet implements API.ArgsGet
func (w *api) ArgsGet(ctx *wasm.HostFunctionCallContext, argv, argvBuf uint32) Errno {
	if !ctx.Memory.ValidateAddrRange(argv, uint64(len(w.args.nullTerminatedValues))*4) /*4 is the size of uint32*/ ||
		!ctx.Memory.ValidateAddrRange(argvBuf, uint64(w.args.totalBufSize)) {
		return EINVAL
	}
	for _, arg := range w.args.nullTerminatedValues {
		binary.LittleEndian.PutUint32(ctx.Memory.Buffer[argv:], argvBuf)
		argv += 4 // size of uint32
		argvBuf += uint32(copy(ctx.Memory.Buffer[argvBuf:], arg))
	}

	return ESUCCESS
}

func proc_exit(*wasm.HostFunctionCallContext, uint32) {
	// not implemented yet
}

func environ_sizes_get(*wasm.HostFunctionCallContext, uint32, uint32) (err Errno) {
	// not implemented yet
	return
}

func environ_get(*wasm.HostFunctionCallContext, uint32, uint32) (err Errno) {
	return
}

// clock_time_get is a WASI API that returns the time value of a clock. Note: This is similar to clock_gettime in POSIX.
// * id: The clock id for which to return the time.
// * precision: timestamp The maximum lag (exclusive) that the returned time value may have, compared to its actual value.
// * timestampPtr: a pointer to an address of uint64 type. The time value of the clock in nanoseconds is written there.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_time_getid-clockid-precision-timestamp---errno-timestamp
func (w *api) clock_time_get(ctx *wasm.HostFunctionCallContext, id uint32, precision uint64, timestampPtr uint32) (err Errno) {
	// TODO: The clock id and precision are currently ignored.
	if !ctx.Memory.PutUint64(timestampPtr, w.getTimeNanosFn()) {
		return EINVAL
	}
	return ESUCCESS
}
