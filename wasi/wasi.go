package wasi

import (
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"math"
	"math/rand"
	"os"
	"reflect"
	"time"

	"github.com/tetratelabs/wazero/wasm"
)

// API includes all host functions to export for WASI version "wasi_snapshot_preview1"
//
// Note: When translating WASI functions, each result besides Errno is always an uint32 parameter. WebAssembly 1.0 (MVP)
// can have up to one result, which is already used by Errno. This forces other results to be parameters. A result
// parameter is a memory offset to write the result to. As memory offsets are uint32, each parameter representing a
// result is uint32.
//
// Note: In WebAssembly 1.0 (MVP), there may be up to one Memory per store, which means the wasm.HostFunctionCallContext
// Memory is always the wasm.Store Memories index zero: `store.Memories[0].Buffer`
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md
// See https://wwa.w3.org/TR/wasm-core-1/#memory-instances%E2%91%A0.
type API interface {
	// ArgsGet is the WASI function that reads command-line argument data (Args).
	//
	// There are two parameters and an Errno result. Both parameters are offsets in the wasm.HostFunctionCallContext
	// Memory Buffer to write offsets. These are encoded uint32 little-endian.
	//
	// * argv - is the offset to begin writing argument offsets to the wasm.MemoryInstance
	//   * ArgsSizesGet result argc * 4 bytes are written to this offset
	// * argvBuf - is the offset to write the null terminated arguments to the wasm.MemoryInstance
	//   * ArgsSizesGet result argv_buf_size bytes are written to this offset
	//
	// For example, if ArgsSizesGet wrote argc=2 and argvBufSize=5 for arguments: "a" and "bc"
	//   and ArgsGet results argv=7 and argvBuf=1, we expect `ctx.Memory.Buffer` to contain:
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
	// Note: FunctionArgsGet documentation has an example of this signature in the WebAssembly 1.0 (MVP) Text Format.
	// See ArgsGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#args_get
	// See https://en.wikipedia.org/wiki/Null-terminated_string
	ArgsGet(ctx *wasm.HostFunctionCallContext, argv, argvBuf uint32) Errno

	// ArgsSizesGet is a WASI function that reads command-line argument data (Args) sizes.
	//
	// There are two parameters and an Errno result. Both parameters are offsets in the wasm.HostFunctionCallContext
	// Memory Buffer to write the corresponding sizes in uint32 little-endian encoding.
	//
	// * argc - is the offset to write the argument count to the wasm.MemoryInstance Buffer
	// * argvBufSize - is the offset to write the null terminated argument length to the wasm.MemoryInstance Buffer
	//
	// For example, if Args are []string{"a","bc"} and
	//   ArgsSizesGet parameters resultArgc=1 and resultArgvBufSize=6, we expect `ctx.Memory.Buffer` to contain:
	//
	//                    uint32         uint32
	//                  +--------+     +--------+
	//                  |        |     |        |
	//        []byte{?, 2, 0, 0, 0, ?, 5, 0, 0, 0, ?}
	//     resultArgc --^              ^
	//         2 args --+              |
	//             resultArgvBufSize --|
	//   len([]byte{'a',0,'b',c',0}) --+
	//
	// Note: FunctionArgsSizesGet documentation has an example of this signature in the WebAssembly 1.0 (MVP) Text Format.
	// See ArgsSizesGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#args_sizes_get
	// See https://en.wikipedia.org/wiki/Null-terminated_string
	ArgsSizesGet(ctx *wasm.HostFunctionCallContext, resultArgc, resultArgvBufSize uint32) Errno

	// TODO: EnvironGet(ctx *wasm.HostFunctionCallContext, environ, environBuf uint32) Errno

	// TODO: EnvironSizesGet(ctx *wasm.HostFunctionCallContext, resulEnvironc, resultEnvironBufSize uint32) Errno

	// TODO: ClockResGet(ctx *wasm.HostFunctionCallContext, id, resultResolution uint32) Errno

	// TODO: ClockTimeGet(ctx *wasm.HostFunctionCallContext, id uint32, precision uint64, resultTimestamp uint32) Errno

	// TODO: FDAdvise
	// TODO: FDAllocate
	// TODO: FDClose
	// TODO: FDDataSync
	// TODO: FDFDStatGet
	// TODO: FDFDStatSetFlags
	// TODO: FDFDStatSetRights
	// TODO: FDFilestatGet
	// TODO: FDFilestatSetSize
	// TODO: FDFilestatSetTimes
	// TODO: FDPread
	// TODO: FDPrestatGet
	// TODO: FDPrestatDirName
	// TODO: FDPwrite
	// TODO: FDRead
	// TODO: FDReaddir
	// TODO: FDRenumber
	// TODO: FDSeek
	// TODO: FDSync
	// TODO: FDTell
	// TODO: FDWrite
	// TODO: PathCreateDirectory
	// TODO: PathFilestatGet
	// TODO: PathFilestatSetTimes
	// TODO: PathLink
	// TODO: PathOpen
	// TODO: PathReadlink
	// TODO: PathRemoveDirectory
	// TODO: PathRename
	// TODO: PathSymlink
	// TODO: PathUnlinkFile
	// TODO: PollOneoff
	// TODO: ProcExit
	// TODO: ProcRaise
	// TODO: SchedYield
	// TODO: RandomGet
	// TODO: SockRecv
	// TODO: SockSend
	// TODO: SockShutdown
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

func (a *api) register(store *wasm.Store) (err error) {
	// Note: these are ordered per spec for consistency
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#functions
	nameToFunction := []struct {
		funcName string
		fn       interface{}
	}{
		{FunctionArgsGet, a.ArgsGet},
		{FunctionArgsSizesGet, a.ArgsSizesGet},
		{FunctionEnvironGet, environ_get},
		{FunctionEnvironSizesGet, environ_sizes_get},
		// TODO: FunctionClockResGet
		{FunctionClockTimeGet, a.clock_time_get},
		// TODO: FunctionFDAdvise
		// TODO: FunctionFDAllocate
		{FunctionFDClose, a.fd_close},
		// TODO: FunctionFDDataSync
		{FunctionFDFDStatGet, a.fd_fdstat_get},
		// TODO: FunctionFDFDStatSetFlags
		// TODO: FunctionFDFDStatSetRights
		// TODO: FunctionFDFilestatGet
		// TODO: FunctionFDFilestatSetSize
		// TODO: FunctionFDFilestatSetTimes
		// TODO: FunctionFDPread
		{FunctionFDPrestatGet, a.fd_prestat_get},
		{FunctionFDPrestatDirName, a.fd_prestat_dir_name},
		// TODO: FunctionFDPwrite
		{FunctionFDRead, a.fd_read},
		// TODO: FunctionFDReaddir
		// TODO: FunctionFDRenumber
		{FunctionFDSeek, a.fd_seek},
		// TODO: FunctionFDSync
		// TODO: FunctionFDTell
		{FunctionFDWrite, a.fd_write},
		// TODO: FunctionPathCreateDirectory
		// TODO: FunctionPathFilestatGet
		// TODO: FunctionPathFilestatSetTimes
		// TODO: FunctionPathLink
		{FunctionPathOpen, a.path_open},
		// TODO: FunctionPathReadlink
		// TODO: FunctionPathRemoveDirectory
		// TODO: FunctionPathRename
		// TODO: FunctionPathSymlink
		// TODO: FunctionPathUnlinkFile
		// TODO: FunctionPollOneoff
		{FunctionProcExit, proc_exit},
		// TODO: FunctionProcRaise
		// TODO: FunctionSchedYield
		// TODO: FunctionRandomGet
		// TODO: FunctionSockRecv
		// TODO: FunctionSockSend
		// TODO: FunctionSockShutdown
	}
	for _, wasiName := range []string{
		wasiUnstableName, // TODO: check if there are any signature incompatibility between stable and preview 1
		wasiSnapshotPreview1Name,
	} {
		for _, pair := range nameToFunction {
			err = store.AddHostFunction(wasiName, pair.funcName, reflect.ValueOf(pair.fn))
			if err != nil {
				return err
			}
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
	return func(a *api) {
		a.stdin = reader
	}
}

func Stdout(writer io.Writer) Option {
	return func(a *api) {
		a.stdout = writer
	}
}

func Stderr(writer io.Writer) Option {
	return func(a *api) {
		a.stderr = writer
	}
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
	return func(a *api) {
		a.args = wasiStrings
	}, nil
}

func Preopen(dir string, fileSys FS) Option {
	return func(a *api) {
		a.opened[uint32(len(a.opened))+3] = fileEntry{
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

func (a *api) randUnusedFD() uint32 {
	fd := uint32(rand.Int31())
	for {
		if _, ok := a.opened[fd]; !ok {
			return fd
		}
		fd = (fd + 1) % (1 << 31)
	}
}

func (a *api) fd_prestat_get(ctx *wasm.HostFunctionCallContext, fd uint32, bufPtr uint32) (err Errno) {
	if _, ok := a.opened[fd]; !ok {
		return ErrnoBadf
	}
	return ErrnoSuccess
}

func (a *api) fd_prestat_dir_name(ctx *wasm.HostFunctionCallContext, fd uint32, pathPtr uint32, pathLen uint32) (err Errno) {
	f, ok := a.opened[fd]
	if !ok {
		return ErrnoInval
	}

	if uint32(len(f.path)) < pathLen {
		return ErrnoNametoolong
	}

	copy(ctx.Memory.Buffer[pathPtr:], f.path)
	return ErrnoSuccess
}

func (a *api) fd_fdstat_get(ctx *wasm.HostFunctionCallContext, fd uint32, bufPtr uint32) (err Errno) {
	if _, ok := a.opened[fd]; !ok {
		return ErrnoBadf
	}
	binary.LittleEndian.PutUint64(ctx.Memory.Buffer[bufPtr+16:], R_FD_READ|R_FD_WRITE)
	return ErrnoSuccess
}

func (a *api) path_open(ctx *wasm.HostFunctionCallContext, fd, dirFlags, pathPtr, pathLen, oFlags uint32,
	fsRightsBase, fsRightsInheriting uint64,
	fdFlags, fdPtr uint32) (errno Errno) {
	dir, ok := a.opened[fd]
	if !ok || dir.fileSys == nil {
		return ErrnoInval
	}

	path := string(ctx.Memory.Buffer[pathPtr : pathPtr+pathLen])
	f, err := dir.fileSys.OpenWASI(dirFlags, path, oFlags, fsRightsBase, fsRightsInheriting, fdFlags)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return ErrnoNoent
		default:
			return ErrnoInval
		}
	}

	newFD := a.randUnusedFD()

	a.opened[newFD] = fileEntry{
		file: f,
	}

	binary.LittleEndian.PutUint32(ctx.Memory.Buffer[fdPtr:], newFD)
	return ErrnoSuccess
}

func (a *api) fd_seek(ctx *wasm.HostFunctionCallContext, fd uint32, offset uint64, whence uint32, nwrittenPtr uint32) (err Errno) {
	return ErrnoNosys // TODO: implement
}

func (a *api) fd_write(ctx *wasm.HostFunctionCallContext, fd uint32, iovsPtr uint32, iovsLen uint32, nwrittenPtr uint32) (err Errno) {
	var writer io.Writer

	switch fd {
	case 1:
		writer = a.stdout
	case 2:
		writer = a.stderr
	default:
		f, ok := a.opened[fd]
		if !ok || f.file == nil {
			return ErrnoBadf
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
	return ErrnoSuccess
}

func (a *api) fd_read(ctx *wasm.HostFunctionCallContext, fd uint32, iovsPtr uint32, iovsLen uint32, nreadPtr uint32) (err Errno) {
	var reader io.Reader

	switch fd {
	case 0:
		reader = a.stdin
	default:
		f, ok := a.opened[fd]
		if !ok || f.file == nil {
			return ErrnoBadf
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
			return ErrnoIo
		}
	}
	binary.LittleEndian.PutUint32(ctx.Memory.Buffer[nreadPtr:], nread)
	return ErrnoSuccess
}

func (a *api) fd_close(ctx *wasm.HostFunctionCallContext, fd uint32) (err Errno) {
	f, ok := a.opened[fd]
	if !ok {
		return ErrnoBadf
	}

	if f.file != nil {
		f.file.Close()
	}

	delete(a.opened, fd)

	return ErrnoSuccess
}

// ArgsSizesGet implements API.ArgsSizesGet
func (a *api) ArgsSizesGet(ctx *wasm.HostFunctionCallContext, resultArgc, resultArgvBufSize uint32) Errno {
	if !ctx.Memory.PutUint32(resultArgc, uint32(len(a.args.nullTerminatedValues))) {
		return ErrnoInval
	}
	if !ctx.Memory.PutUint32(resultArgvBufSize, a.args.totalBufSize) {
		return ErrnoInval
	}

	return ErrnoSuccess
}

// ArgsGet implements API.ArgsGet
func (a *api) ArgsGet(ctx *wasm.HostFunctionCallContext, argv, argvBuf uint32) Errno {
	if !ctx.Memory.ValidateAddrRange(argv, uint64(len(a.args.nullTerminatedValues))*4) /*4 is the size of uint32*/ ||
		!ctx.Memory.ValidateAddrRange(argvBuf, uint64(a.args.totalBufSize)) {
		return ErrnoInval
	}
	for _, arg := range a.args.nullTerminatedValues {
		binary.LittleEndian.PutUint32(ctx.Memory.Buffer[argv:], argvBuf)
		argv += 4 // size of uint32
		argvBuf += uint32(copy(ctx.Memory.Buffer[argvBuf:], arg))
	}

	return ErrnoSuccess
}

func proc_exit(*wasm.HostFunctionCallContext, uint32) {
	// TODO: implement
}

func environ_sizes_get(*wasm.HostFunctionCallContext, uint32, uint32) (err Errno) {
	return ErrnoNosys // TODO: implement
}

func environ_get(*wasm.HostFunctionCallContext, uint32, uint32) (err Errno) {
	return ErrnoNosys // TODO: implement
}

// clock_time_get is a WASI API that returns the time value of a clock. Note: This is similar to clock_gettime in POSIX.
// * id: The clock id for which to return the time.
// * precision: timestamp The maximum lag (exclusive) that the returned time value may have, compared to its actual value.
// * timestampPtr: a pointer to an address of uint64 type. The time value of the clock in nanoseconds is written there.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_time_getid-clockid-precision-timestamp---errno-timestamp
func (a *api) clock_time_get(ctx *wasm.HostFunctionCallContext, id uint32, precision uint64, timestampPtr uint32) (err Errno) {
	// TODO: The clock id and precision are currently ignored.
	if !ctx.Memory.PutUint64(timestampPtr, a.getTimeNanosFn()) {
		return ErrnoInval
	}
	return ErrnoSuccess
}
