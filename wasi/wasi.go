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

	"github.com/tetratelabs/wazero/wasm"
)

const (
	wasiUnstableName         = "wasi_unstable"
	wasiSnapshotPreview1Name = "wasi_snapshot_preview1"
)

type WASIEnvironment struct {
	args  *wasiStringArray
	stdin io.Reader
	stdout,
	stderr io.Writer
	opened map[uint32]fileEntry
}

func (w *WASIEnvironment) Register(store *wasm.Store) (err error) {
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
		err = store.AddHostFunction(wasiName, "path_open", reflect.ValueOf(w.path_open))
		if err != nil {
			return err
		}
		err = store.AddHostFunction(wasiName, "args_get", reflect.ValueOf(w.args_get))
		if err != nil {
			return err
		}
		err = store.AddHostFunction(wasiName, "args_sizes_get", reflect.ValueOf(w.args_sizes_get))
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

type Option func(*WASIEnvironment)

func Stdin(reader io.Reader) Option {
	return func(w *WASIEnvironment) {
		w.stdin = reader
	}
}

func Stdout(writer io.Writer) Option {
	return func(w *WASIEnvironment) {
		w.stdout = writer
	}
}

func Stderr(writer io.Writer) Option {
	return func(w *WASIEnvironment) {
		w.stderr = writer
	}
}

// wasiStringArray holds null-terminated strings. It ensures that
// its length and total buffer size don't exceed the max of uint32.
// Each string can have arbitrary byte values, not only utf-8 encoded text.
// wasiStringArray are convenience struct for args_get and environ_get. (environ_get is not implemented yet)
//
// A Null-terminated string is a byte string with a NULL suffix ("\x00").
// Link: https://en.wikipedia.org/wiki/Null-terminated_string
type wasiStringArray struct {
	// nullTerminatedValues are null-terminated values with a NULL suffix.
	// Each string can have arbitrary byte values, not only utf-8 encoded text.
	nullTerminatedValues [][]byte
	totalBufSize         uint32
}

// newWASIStringArray creates a wasiStringArray from the given string slice. It returns an error
// if the length or the total buffer size of the result WASIStringArray exceeds the max of uint32
func newWASIStringArray(args []string) (*wasiStringArray, error) {
	if args == nil {
		return &wasiStringArray{nullTerminatedValues: [][]byte{}}, nil
	}
	if len(args) > math.MaxUint32 {
		return nil, fmt.Errorf("the length of the args exceeds the max of uint32: %v", len(args))
	}
	strings := make([][]byte, len(args))
	totalBufSize := uint32(0)
	for i, arg := range args {
		argLen := uint64(len(arg)) + 1 // + 1 for "\x00"
		if argLen > uint64(math.MaxUint32-totalBufSize) {
			return nil, fmt.Errorf("the required buffer size for the args exceeds the max of uint32: %v", uint64(totalBufSize)+argLen)
		}
		totalBufSize += uint32(argLen)
		strings[i] = make([]byte, argLen)
		copy(strings[i], arg)
		strings[i][argLen-1] = byte(0)
	}

	return &wasiStringArray{nullTerminatedValues: strings, totalBufSize: totalBufSize}, nil
}

func Args(args []string) (Option, error) {
	wasiStrings, err := newWASIStringArray(args)
	if err != nil {
		return nil, err
	}
	return func(w *WASIEnvironment) {
		w.args = wasiStrings
	}, nil
}

func Preopen(dir string, fileSys FS) Option {
	return func(w *WASIEnvironment) {
		w.opened[uint32(len(w.opened))+3] = fileEntry{
			path:    dir,
			fileSys: fileSys,
		}
	}
}

func NewEnvironment(opts ...Option) *WASIEnvironment {
	ret := &WASIEnvironment{
		args:   &wasiStringArray{},
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
		opened: map[uint32]fileEntry{},
	}

	// apply functional options
	for _, f := range opts {
		f(ret)
	}

	return ret
}

func (w *WASIEnvironment) randUnusedFD() uint32 {
	fd := uint32(rand.Int31())
	for {
		if _, ok := w.opened[fd]; !ok {
			return fd
		}
		fd = (fd + 1) % (1 << 31)
	}
}

func (w *WASIEnvironment) fd_prestat_get(ctx *wasm.HostFunctionCallContext, fd uint32, bufPtr uint32) (err Errno) {
	if _, ok := w.opened[fd]; !ok {
		return EBADF
	}
	return ESUCCESS
}

func (w *WASIEnvironment) fd_prestat_dir_name(ctx *wasm.HostFunctionCallContext, fd uint32, pathPtr uint32, pathLen uint32) (err Errno) {
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

func (w *WASIEnvironment) fd_fdstat_get(ctx *wasm.HostFunctionCallContext, fd uint32, bufPtr uint32) (err Errno) {
	if _, ok := w.opened[fd]; !ok {
		return EBADF
	}
	binary.LittleEndian.PutUint64(ctx.Memory.Buffer[bufPtr+16:], R_FD_READ|R_FD_WRITE)
	return ESUCCESS
}

func (w *WASIEnvironment) path_open(ctx *wasm.HostFunctionCallContext, fd, dirFlags, pathPtr, pathLen, oFlags uint32,
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

func (w *WASIEnvironment) fd_write(ctx *wasm.HostFunctionCallContext, fd uint32, iovsPtr uint32, iovsLen uint32, nwrittenPtr uint32) (err Errno) {
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

func (w *WASIEnvironment) fd_read(ctx *wasm.HostFunctionCallContext, fd uint32, iovsPtr uint32, iovsLen uint32, nreadPtr uint32) (err Errno) {
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

func (w *WASIEnvironment) fd_close(ctx *wasm.HostFunctionCallContext, fd uint32) (err Errno) {
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

// args_sizes_get is a WASI API that returns the number of the command-line arguments and the total buffer size that
// args_get API will require to store the value of the command-line arguments.
// * argsCountPtr: a pointer to an address of uint32 type. The number of the command-line arguments is written there.
// * argsBufSizePtr: a pointer to an address of uint32 type. The number of the command-line arguments is written there.
//
// Link to the actual spec: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-args_sizes_get---errno-size-size
func (w *WASIEnvironment) args_sizes_get(ctx *wasm.HostFunctionCallContext, argsCountPtr uint32, argsBufSizePtr uint32) Errno {
	if !ctx.Memory.PutUint32(argsCountPtr, uint32(len(w.args.nullTerminatedValues))) {
		return EINVAL
	}
	if !ctx.Memory.PutUint32(argsBufSizePtr, w.args.totalBufSize) {
		return EINVAL
	}

	return ESUCCESS
}

// args_get is a WASI API to read the command-line argument data.
// * argsPtr:
//     A pointer to a buffer. args_get writes multiple *C.char pointers in sequence there. In other words, this is an array of *char in C.
//     Each *C.char of them points to a command-line argument that is a null-terminated string.
//     The number of this *C.char matches the value that args_sizes_get returns in argsCountPtr.
// * argsBufPtr:
//     A pointer to a buffer. args_get writes the command line arguments as null-terminated strings in the given buffer.
//     The total number of bytes written there is the value that args_sizes_get returns in argsBufSizePtr. The caller must ensure that
//     the buffer has the enough size.
//     Each *C.char pointer that can be obtained from argsPtr points to the beginning of each of these null-terminated strings.
// Link to the actual spec: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-args_getargv-pointerpointeru8-argv_buf-pointeru8---errno
// Reference: https://en.wikipedia.org/wiki/Null-terminated_string
func (w *WASIEnvironment) args_get(ctx *wasm.HostFunctionCallContext, argsPtr uint32, argsBufPtr uint32) (err Errno) {
	if !ctx.Memory.ValidateAddrRange(argsPtr, uint64(len(w.args.nullTerminatedValues))*4) /*4 is the size of uint32*/ ||
		!ctx.Memory.ValidateAddrRange(argsBufPtr, uint64(w.args.totalBufSize)) {
		return EINVAL
	}
	for _, arg := range w.args.nullTerminatedValues {
		binary.LittleEndian.PutUint32(ctx.Memory.Buffer[argsPtr:], argsBufPtr)
		argsPtr += 4 // size of uint32
		argsBufPtr += uint32(copy(ctx.Memory.Buffer[argsBufPtr:], arg))
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
