package internalwasi

import (
	crand "crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	mrand "math/rand"
	"os"
	"strings"
	"time"

	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
)

const (
	// FunctionStart is the name of the nullary function a module must export if it is a WASI Command Module.
	//
	// Note: When this is exported FunctionInitialize must not be.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/design/application-abi.md#current-unstable-abi
	FunctionStart = "_start"

	// FunctionInitialize is the name of the nullary function a module must export if it is a WASI Reactor Module.
	//
	// Note: When this is exported FunctionStart must not be.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/design/application-abi.md#current-unstable-abi
	FunctionInitialize = "_initialize"

	// FunctionArgsGet reads command-line argument data.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-args_getargv-pointerpointeru8-argv_buf-pointeru8---errno
	FunctionArgsGet = "args_get"

	// ImportArgsGet is the WebAssembly 1.0 (MVP) Text format import of FunctionArgsGet
	ImportArgsGet = `(import "wasi_snapshot_preview1" "args_get"
    (func $wasi.args_get (param $argv i32) (param $argv_buf i32) (result (;errno;) i32)))`

	// FunctionArgsSizesGet returns command-line argument data sizes.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-args_sizes_get---errno-size-size
	FunctionArgsSizesGet = "args_sizes_get"

	// ImportArgsSizesGet is the WebAssembly 1.0 (MVP) Text format import of FunctionArgsSizesGet
	ImportArgsSizesGet = `(import "wasi_snapshot_preview1" "args_sizes_get"
    (func $wasi.args_sizes_get (param $result.argc i32) (param $result.argv_buf_size i32) (result (;errno;) i32)))`

	// FunctionEnvironGet reads environment variable data.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-environ_getenviron-pointerpointeru8-environ_buf-pointeru8---errno
	FunctionEnvironGet = "environ_get"

	// ImportEnvironGet is the WebAssembly 1.0 (MVP) Text format import of FunctionEnvironGet
	ImportEnvironGet = `(import "wasi_snapshot_preview1" "environ_get"
    (func $wasi.environ_get (param $environ i32) (param $environ_buf i32) (result (;errno;) i32)))`

	// FunctionEnvironSizesGet returns environment variable data sizes.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-environ_sizes_get---errno-size-size
	FunctionEnvironSizesGet = "environ_sizes_get"

	// ImportEnvironSizesGet is the WebAssembly 1.0 (MVP) Text format import of FunctionEnvironSizesGet
	ImportEnvironSizesGet = `
(import "wasi_snapshot_preview1" "environ_sizes_get"
    (func $wasi.environ_sizes_get (param $result.environc i32) (param $result.environBufSize i32) (result (;errno;) i32)))`

	// FunctionClockResGet returns the resolution of a clock.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_res_getid-clockid---errno-timestamp
	FunctionClockResGet = "clock_res_get"

	// ImportClockResGet is the WebAssembly 1.0 (MVP) Text format import of FunctionClockResGet
	ImportClockResGet = `
(import "wasi_snapshot_preview1" "clock_res_get"
    (func $wasi.clock_res_get (param $id i32) (param $result.resolution i32) (result (;errno;) i32)))`

	// FunctionClockTimeGet returns the time value of a clock.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_time_getid-clockid-precision-timestamp---errno-timestamp
	FunctionClockTimeGet = "clock_time_get"

	// ImportClockTimeGet is the WebAssembly 1.0 (MVP) Text format import of FunctionClockTimeGet
	ImportClockTimeGet = `(import "wasi_snapshot_preview1" "clock_time_get"
    (func $wasi.clock_time_get (param $id i32) (param $precision i64) (param $result.timestamp i32) (result (;errno;) i32)))`

	FunctionFdAdvise             = "fd_advise"
	FunctionFdAllocate           = "fd_allocate"
	FunctionFdClose              = "fd_close"
	FunctionFdDataSync           = "fd_datasync"
	FunctionFdFdstatGet          = "fd_fdstat_get"
	FunctionFdFdstatSetFlags     = "fd_fdstat_set_flags"
	FunctionFdFdstatSetRights    = "fd_fdstat_set_rights"
	FunctionFdFilestatGet        = "fd_filestat_get"
	FunctionFdFilestatSetSize    = "fd_filestat_set_size"
	FunctionFdFilestatSetTimes   = "fd_filestat_set_times"
	FunctionFdPread              = "fd_pread"
	FunctionFdPrestatGet         = "fd_prestat_get"
	FunctionFdPrestatDirName     = "fd_prestat_dir_name"
	FunctionFdPwrite             = "fd_pwrite"
	FunctionFdRead               = "fd_read"
	FunctionFdReaddir            = "fd_readdir"
	FunctionFdRenumber           = "fd_renumber"
	FunctionFdSeek               = "fd_seek"
	FunctionFdSync               = "fd_sync"
	FunctionFdTell               = "fd_tell"
	FunctionFdWrite              = "fd_write"
	FunctionPathCreateDirectory  = "path_create_directory"
	FunctionPathFilestatGet      = "path_filestat_get"
	FunctionPathFilestatSetTimes = "path_filestat_set_times"
	FunctionPathLink             = "path_link"
	FunctionPathOpen             = "path_open"
	FunctionPathReadlink         = "path_readlink"
	FunctionPathRemoveDirectory  = "path_remove_directory"
	FunctionPathRename           = "path_rename"
	FunctionPathSymlink          = "path_symlink"
	FunctionPathUnlinkFile       = "path_unlink_file"
	FunctionPollOneoff           = "poll_oneoff"
	FunctionProcExit             = "proc_exit"
	FunctionProcRaise            = "proc_raise"
	FunctionSchedYield           = "sched_yield"

	// FunctionRandomGet write random data in buffer
	// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-random_getbuf-pointeru8-buf_len-size---errno
	FunctionRandomGet = "random_get"

	// ImportRandomGet is the WebAssembly 1.0 (MVP) Text format import of FunctionRandomGet
	ImportRandomGet = `(import "wasi_snapshot_preview1" "random_get"
    (func $wasi.random_get (param $buf i32) (param $buf_len i32) (result (;errno;) i32)))`

	FunctionSockRecv     = "sock_recv"
	FunctionSockSend     = "sock_send"
	FunctionSockShutdown = "sock_shutdown"
)

// SnapshotPreview1 includes all host functions to export for WASI version wasi.ModuleSnapshotPreview1.
//
// Note: When translating WASI functions, each result besides Errno is always an uint32 parameter. WebAssembly 1.0 (MVP)
// can have up to one result, which is already used by Errno. This forces other results to be parameters. A result
// parameter is a memory offset to write the result to. As memory offsets are uint32, each parameter representing a
// result is uint32.
//
// Note: Errno mappings are not defined in WASI, yet, so these mappings are best efforts by maintainers.
//
// Note: In WebAssembly 1.0 (MVP), there may be up to one Memory per store, which means wasm.Memory is always the
// wasm.Store Memories index zero: `store.Memories[0].Buffer`
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md
// See https://github.com/WebAssembly/WASI/issues/215
// See https://wwa.w3.org/TR/wasm-core-1/#memory-instances%E2%91%A0.
type SnapshotPreview1 interface {
	// ArgsGet is the WASI function that reads command-line argument data (Args).
	//
	// There are two parameters. Both are offsets in wasm.HostFunctionCallContext Memory. If either are invalid due to
	// memory constraints, this returns ErrnoFault.
	//
	// * argv - is the offset to begin writing argument offsets in uint32 little-endian encoding.
	//   * ArgsSizesGet result argc * 4 bytes are written to this offset
	// * argvBuf - is the offset to write the null terminated arguments to wasm.MemoryInstance
	//   * ArgsSizesGet result argv_buf_size bytes are written to this offset
	//
	// For example, if ArgsSizesGet wrote argc=2 and argvBufSize=5 for arguments: "a" and "bc"
	//   and ArgsGet results argv=7 and argvBuf=1, we expect `ctx.Memory` to contain:
	//
	//               argvBufSize          uint32le    uint32le
	//            +----------------+     +--------+  +--------+
	//            |                |     |        |  |        |
	// []byte{?, 'a', 0, 'b', 'c', 0, ?, 1, 0, 0, 0, 3, 0, 0, 0, ?}
	//  argvBuf --^                      ^           ^
	//                            argv --|           |
	//          offset that begins "a" --+           |
	//                     offset that begins "bc" --+
	//
	// Note: ImportArgsGet shows this signature in the WebAssembly 1.0 (MVP) Text Format.
	// See ArgsSizesGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#args_get
	// See https://en.wikipedia.org/wiki/Null-terminated_string
	ArgsGet(ctx wasm.HostFunctionCallContext, argv, argvBuf uint32) wasi.Errno

	// ArgsSizesGet is the WASI function named FunctionArgsSizesGet that reads command-line argument data (Args)
	// sizes.
	//
	// There are two result parameters: these are offsets in the wasm.HostFunctionCallContext Memory to write
	// corresponding sizes in uint32 little-endian encoding. If either are invalid due to memory constraints, this
	// returns ErrnoFault.
	//
	// * resultArgc - is the offset to write the argument count to wasm.Memory
	// * resultArgvBufSize - is the offset to write the null-terminated argument length to wasm.Memory
	//
	// For example, if Args are []string{"a","bc"} and
	//   ArgsSizesGet parameters resultArgc=1 and resultArgvBufSize=6, we expect `ctx.Memory` to contain:
	//
	//                   uint32le       uint32le
	//                  +--------+     +--------+
	//                  |        |     |        |
	//        []byte{?, 2, 0, 0, 0, ?, 5, 0, 0, 0, ?}
	//     resultArgc --^              ^
	//         2 args --+              |
	//             resultArgvBufSize --|
	//   len([]byte{'a',0,'b',c',0}) --+
	//
	// Note: ImportArgsSizesGet shows this signature in the WebAssembly 1.0 (MVP) Text Format.
	// See ArgsGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#args_sizes_get
	// See https://en.wikipedia.org/wiki/Null-terminated_string
	ArgsSizesGet(ctx wasm.HostFunctionCallContext, resultArgc, resultArgvBufSize uint32) wasi.Errno

	// EnvironGet is the WASI function named FunctionEnvironGet that reads environment variables. (Environ)
	//
	// There are two parameters. Both are offsets in wasm.HostFunctionCallContext Memory. If either are invalid due to
	// memory constraints, this returns ErrnoFault.
	//
	// * environ - is the offset to begin writing environment variables offsets in uint32 little-endian encoding.
	//   * EnvironSizesGet result environc * 4 bytes are written to this offset
	// * environBuf - is the offset to write the environment variables to wasm.Memory
	//   * the format is the same as os.Environ, null terminated "key=val" entries
	//   * EnvironSizesGet result environBufSize bytes are written to this offset
	//
	// For example, if EnvironSizesGet wrote environc=2 and environBufSize=9 for environment variables: "a=b", "b=cd"
	//   and EnvironGet parameters environ=11 and environBuf=1, we expect `ctx.Memory` to contain:
	//
	//                           environBufSize                 uint32le    uint32le
	//              +------------------------------------+     +--------+  +--------+
	//              |                                    |     |        |  |        |
	//   []byte{?, 'a', '=', 'b', 0, 'b', '=', 'c', 'd', 0, ?, 1, 0, 0, 0, 5, 0, 0, 0, ?}
	// environBuf --^                                          ^           ^
	//                              environ offset for "a=b" --+           |
	//                                         environ offset for "b=cd" --+
	//
	// Note: ImportEnvironGet shows this signature in the WebAssembly 1.0 (MVP) Text Format.
	// See EnvironSizesGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#environ_get
	// See https://en.wikipedia.org/wiki/Null-terminated_string
	EnvironGet(ctx wasm.HostFunctionCallContext, environ, environBuf uint32) wasi.Errno

	// EnvironSizesGet is the WASI function named FunctionEnvironSizesGet that reads environment variable
	// (Environ) sizes.
	//
	// There are two result parameters: these are offsets in the wasi.HostFunctionCallContext Memory to write
	// corresponding sizes in uint32 little-endian encoding. If either are invalid due to memory constraints, this
	// returns ErrnoFault.
	//
	// * resultEnvironc - is the offset to write the environment variable count to wasm.Memory
	// * resultEnvironBufSize - is the offset to write the null-terminated environment variable length to wasm.Memory
	//
	// For example, if Environ is []string{"a=b","b=cd"} and
	//   EnvironSizesGet parameters are resultEnvironc=1 and resultEncironBufSize=6, we expect `ctx.Memory` to contain:
	//
	//                   uint32le       uint32le
	//                  +--------+     +--------+
	//                  |        |     |        |
	//        []byte{?, 2, 0, 0, 0, ?, 9, 0, 0, 0, ?}
	// resultEnvironc --^              ^
	//    2 variables --+              |
	//          resultEnvironBufSize --|
	//    len([]byte{'a','=','b',0,    |
	//           'b','=','c','d',0}) --+
	//
	// Note: ImportEnvironGet shows this signature in the WebAssembly 1.0 (MVP) Text Format.
	// See EnvironGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#environ_sizes_get
	// See https://en.wikipedia.org/wiki/Null-terminated_string
	EnvironSizesGet(ctx wasm.HostFunctionCallContext, resultEnvironc, resultEnvironBufSize uint32) wasi.Errno

	// TODO: ClockResGet(ctx wasm.HostFunctionCallContext, id, resultResolution uint32) wasi.Errno

	// ClockTimeGet is the WASI function named FunctionClockTimeGet that returns the time value of a clock (time.Now).
	//
	// * id - The clock id for which to return the time.
	// * precision - The maximum lag (exclusive) that the returned time value may have, compared to its actual value.
	// * resultTimestamp - the offset to write the timestamp to wasm.Memory
	//   * the timestamp is epoch nanoseconds encoded as a uint64 little-endian encoding.
	//
	// For example, if time.Now returned exactly midnight UTC 2022-01-01 (1640995200000000000), and
	//   ClockTimeGet resultTimestamp=1, we expect `ctx.Memory` to contain:
	//
	//                                      uint64le
	//                    +------------------------------------------+
	//                    |                                          |
	//          []byte{?, 0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, ?}
	//  resultTimestamp --^
	//
	// Note: ImportClockTimeGet shows this signature in the WebAssembly 1.0 (MVP) Text Format.
	// Note: This is similar to `clock_gettime` in POSIX.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_time_getid-clockid-precision-timestamp---errno-timestamp
	// See https://linux.die.net/man/3/clock_gettime
	ClockTimeGet(ctx wasm.HostFunctionCallContext, id uint32, precision uint64, resultTimestamp uint32) wasi.Errno

	// TODO: wasi.FdAdvise
	// TODO: wasi.FdAllocate
	// TODO: wasi.FdClose
	// TODO: wasi.FdDataSync
	// TODO: wasi.FdFdstatGet
	// TODO: wasi.FdFdstatSetFlags
	// TODO: wasi.FdFdstatSetRights
	// TODO: wasi.FdFilestatGet
	// TODO: wasi.FdFilestatSetSize
	// TODO: wasi.FdFilestatSetTimes
	// TODO: wasi.FdPread
	// TODO: wasi.FdPrestatGet
	// TODO: wasi.FdPrestatDirName
	// TODO: wasi.FdPwrite
	// TODO: wasi.FdRead
	// TODO: wasi.FdReaddir
	// TODO: wasi.FdRenumber
	// TODO: wasi.FdSeek
	// TODO: wasi.FdSync
	// TODO: wasi.FdTell
	// TODO: wasi.FdWrite
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

	// RandomGet is the WASI function named FunctionRandomGet that write random data in buffer (rand.Read()).
	//
	// * buf - is the wasm.Memory offset to write random values
	// * bufLen - size of random data in bytes
	//
	// For example, if underlying random source was seeded like `rand.NewSource(42)`, we expect `ctx.Memory` to contain:
	//
	//                             bufLen (5)
	//                    +--------------------------+
	//                    |                        	 |
	//          []byte{?, 0x53, 0x8c, 0x7f, 0x96, 0xb1, ?}
	//              buf --^
	//
	// Note: ImportRandomGet shows this signature in the WebAssembly 1.0 (MVP) Text Format.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-random_getbuf-pointeru8-bufLen-size---errno
	RandomGet(ctx wasm.HostFunctionCallContext, buf, bufLen uint32) wasi.Errno

	// TODO: SockRecv
	// TODO: SockSend
	// TODO: SockShutdown
}

type wasiAPI struct {
	args *nullTerminatedStrings
	// environ stores each environment variable in the form of "key=value",
	// which is both convenient for the implementation of environ_get and matches os.Environ
	environ *nullTerminatedStrings
	stdin   io.Reader
	stdout,
	stderr io.Writer
	opened map[uint32]fileEntry
	// timeNowUnixNano is mutable for testing
	timeNowUnixNano func() uint64
	randSource      func([]byte) error
}

// SnapshotPreview1Functions returns all go functions that implement SnapshotPreview1.
// These should be exported in the module named wasi.ModuleSnapshotPreview1.
// See wasm.NewHostFunction
// TODO: we can't export a return with SnapshotPreview1 until we figure out how to give users a wasm.HostFunctionCallContext
func SnapshotPreview1Functions(opts ...Option) (a *wasiAPI, nameToGoFunc map[string]interface{}) {
	a = newAPI(opts...)
	// Note: these are ordered per spec for consistency even if the resulting map can't guarantee that.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#functions
	nameToGoFunc = map[string]interface{}{
		FunctionArgsGet:         a.ArgsGet,
		FunctionArgsSizesGet:    a.ArgsSizesGet,
		FunctionEnvironGet:      a.EnvironGet,
		FunctionEnvironSizesGet: a.EnvironSizesGet,
		// TODO: FunctionClockResGet
		FunctionClockTimeGet: a.ClockTimeGet,
		// TODO: FunctionFdAdvise
		// TODO: FunctionFdAllocate
		FunctionFdClose: a.fd_close,
		// TODO: FunctionFdDataSync
		FunctionFdFdstatGet: a.fd_fdstat_get,
		// TODO: FunctionFdFdstatSetFlags
		// TODO: FunctionFdFdstatSetRights
		// TODO: FunctionFdFilestatGet
		// TODO: FunctionFdFilestatSetSize
		// TODO: FunctionFdFilestatSetTimes
		// TODO: FunctionFdPread
		FunctionFdPrestatGet:     a.fd_prestat_get,
		FunctionFdPrestatDirName: a.fd_prestat_dir_name,
		// TODO: FunctionFdPwrite
		FunctionFdRead: a.fd_read,
		// TODO: FunctionFdReaddir
		// TODO: FunctionFdRenumber
		FunctionFdSeek: a.fd_seek,
		// TODO: FunctionFdSync
		// TODO: FunctionFdTell
		FunctionFdWrite: a.fd_write,
		// TODO: FunctionPathCreateDirectory
		// TODO: FunctionPathFilestatGet
		// TODO: FunctionPathFilestatSetTimes
		// TODO: FunctionPathLink
		FunctionPathOpen: a.path_open,
		// TODO: FunctionPathReadlink
		// TODO: FunctionPathRemoveDirectory
		// TODO: FunctionPathRename
		// TODO: FunctionPathSymlink
		// TODO: FunctionPathUnlinkFile
		// TODO: FunctionPollOneoff
		FunctionProcExit: proc_exit,
		// TODO: FunctionProcRaise
		// TODO: FunctionSchedYield
		FunctionRandomGet: a.RandomGet,
		// TODO: FunctionSockRecv
		// TODO: FunctionSockSend
		// TODO: FunctionSockShutdown
	}
	return
}

// ArgsGet implements SnapshotPreview1.ArgsGet
func (a *wasiAPI) ArgsGet(ctx wasm.HostFunctionCallContext, argv, argvBuf uint32) wasi.Errno {
	for _, arg := range a.args.nullTerminatedValues {
		if !ctx.Memory().WriteUint32Le(argv, argvBuf) {
			return wasi.ErrnoFault
		}
		argv += 4 // size of uint32
		if !ctx.Memory().Write(argvBuf, arg) {
			return wasi.ErrnoFault
		}
		argvBuf += uint32(len(arg))
	}

	return wasi.ErrnoSuccess
}

// ArgsSizesGet implements SnapshotPreview1.ArgsSizesGet
func (a *wasiAPI) ArgsSizesGet(ctx wasm.HostFunctionCallContext, resultArgc, resultArgvBufSize uint32) wasi.Errno {
	if !ctx.Memory().WriteUint32Le(resultArgc, uint32(len(a.args.nullTerminatedValues))) {
		return wasi.ErrnoFault
	}
	if !ctx.Memory().WriteUint32Le(resultArgvBufSize, a.args.totalBufSize) {
		return wasi.ErrnoFault
	}
	return wasi.ErrnoSuccess
}

// EnvironGet implements SnapshotPreview1.EnvironGet
func (a *wasiAPI) EnvironGet(ctx wasm.HostFunctionCallContext, environ uint32, environBuf uint32) (err wasi.Errno) {
	// w.environ holds the environment variables in the form of "key=val\x00", so just copies it to the linear memory.
	for _, env := range a.environ.nullTerminatedValues {
		if !ctx.Memory().WriteUint32Le(environ, environBuf) {
			return wasi.ErrnoFault
		}
		environ += 4 // size of uint32
		if !ctx.Memory().Write(environBuf, env) {
			return wasi.ErrnoFault
		}
		environBuf += uint32(len(env))
	}

	return wasi.ErrnoSuccess
}

// EnvironSizesGet implements SnapshotPreview1.EnvironSizesGet
func (a *wasiAPI) EnvironSizesGet(ctx wasm.HostFunctionCallContext, resultEnvironc uint32, resultEnvironBufSize uint32) (err wasi.Errno) {
	if !ctx.Memory().WriteUint32Le(resultEnvironc, uint32(len(a.environ.nullTerminatedValues))) {
		return wasi.ErrnoFault
	}
	if !ctx.Memory().WriteUint32Le(resultEnvironBufSize, a.environ.totalBufSize) {
		return wasi.ErrnoFault
	}

	return wasi.ErrnoSuccess
}

// TODO: Func (a *wasiAPI) FunctionClockResGet

// ClockTimeGet implements SnapshotPreview1.ClockTimeGet
func (a *wasiAPI) ClockTimeGet(ctx wasm.HostFunctionCallContext, id uint32, precision uint64, resultTimestamp uint32) wasi.Errno {
	// TODO: id and precision are currently ignored.
	if !ctx.Memory().WriteUint64Le(resultTimestamp, a.timeNowUnixNano()) {
		return wasi.ErrnoFault
	}
	return wasi.ErrnoSuccess
}

type fileEntry struct {
	path    string
	fileSys wasi.FS
	file    wasi.File
}

type Option func(*wasiAPI)

func Stdin(reader io.Reader) Option {
	return func(a *wasiAPI) {
		a.stdin = reader
	}
}

func Stdout(writer io.Writer) Option {
	return func(a *wasiAPI) {
		a.stdout = writer
	}
}

func Stderr(writer io.Writer) Option {
	return func(a *wasiAPI) {
		a.stderr = writer
	}
}

// Args returns an option to give a command-line arguments in SnapshotPreview1 or errs if the inputs are too large.
//
// Note: The only reason to set this is to control what's written by SnapshotPreview1.ArgsSizesGet and SnapshotPreview1.ArgsGet
// Note: While similar in structure to os.Args, this controls what's visible in Wasm (ex the WASI function "_start").
func Args(args ...string) (Option, error) {
	wasiStrings, err := newNullTerminatedStrings(math.MaxUint32, "arg", args...) // TODO: this is crazy high even if spec allows it
	if err != nil {
		return nil, err
	}
	return func(a *wasiAPI) {
		a.args = wasiStrings
	}, nil
}

// Environ returns an option to set environment variables in SnapshotPreview1.
// Environ returns an error if the input contains a string not joined with `=`, or if the inputs are too large.
//  * environ: environment variables in the same format as that of `os.Environ`, where key/value pairs are joined with `=`.
// See os.Environ
//
// Note: Implicit environment variable propagation into WASI is intentionally not done.
// Note: The only reason to set this is to control what's written by SnapshotPreview1.EnvironSizesGet and SnapshotPreview1.EnvironGet
// Note: While similar in structure to os.Environ, this controls what's visible in Wasm (ex the WASI function "_start").
func Environ(environ ...string) (Option, error) {
	for i, env := range environ {
		if !strings.Contains(env, "=") {
			return nil, fmt.Errorf("environ[%d] is not joined with '='", i)
		}
	}
	wasiStrings, err := newNullTerminatedStrings(math.MaxUint32, "environ", environ...) // TODO: this is crazy high even if spec allows it
	if err != nil {
		return nil, err
	}
	return func(w *wasiAPI) {
		w.environ = wasiStrings
	}, nil
}

func Preopen(dir string, fileSys wasi.FS) Option {
	return func(a *wasiAPI) {
		a.opened[uint32(len(a.opened))+3] = fileEntry{
			path:    dir,
			fileSys: fileSys,
		}
	}
}

func newAPI(opts ...Option) *wasiAPI {
	ret := &wasiAPI{
		args:    &nullTerminatedStrings{},
		environ: &nullTerminatedStrings{},
		stdin:   os.Stdin,
		stdout:  os.Stdout,
		stderr:  os.Stderr,
		opened:  map[uint32]fileEntry{},
		timeNowUnixNano: func() uint64 {
			return uint64(time.Now().UnixNano())
		},
		randSource: func(p []byte) error {
			_, err := crand.Read(p)
			return err
		},
	}

	// apply functional options
	for _, f := range opts {
		f(ret)
	}
	return ret
}

func (a *wasiAPI) randUnusedFD() uint32 {
	fd := uint32(mrand.Int31())
	for {
		if _, ok := a.opened[fd]; !ok {
			return fd
		}
		fd = (fd + 1) % (1 << 31)
	}
}

func (a *wasiAPI) fd_prestat_get(ctx wasm.HostFunctionCallContext, fd uint32, bufPtr uint32) (err wasi.Errno) {
	if _, ok := a.opened[fd]; !ok {
		return wasi.ErrnoBadf
	}
	return wasi.ErrnoSuccess
}

func (a *wasiAPI) fd_prestat_dir_name(ctx wasm.HostFunctionCallContext, fd uint32, pathPtr uint32, pathLen uint32) (err wasi.Errno) {
	f, ok := a.opened[fd]
	if !ok {
		return wasi.ErrnoInval
	}

	if uint32(len(f.path)) < pathLen {
		return wasi.ErrnoNametoolong
	}

	if !ctx.Memory().Write(pathPtr, []byte(f.path)) {
		return wasi.ErrnoFault
	}
	return wasi.ErrnoSuccess
}

func (a *wasiAPI) fd_fdstat_get(ctx wasm.HostFunctionCallContext, fd uint32, bufPtr uint32) (err wasi.Errno) {
	if _, ok := a.opened[fd]; !ok {
		return wasi.ErrnoBadf
	}
	if !ctx.Memory().WriteUint64Le(bufPtr+16, wasi.R_FD_READ|wasi.R_FD_WRITE) {
		return wasi.ErrnoFault
	}
	return wasi.ErrnoSuccess
}

func (a *wasiAPI) path_open(ctx wasm.HostFunctionCallContext, fd, dirFlags, pathPtr, pathLen, oFlags uint32,
	fsRightsBase, fsRightsInheriting uint64,
	fdFlags, fdPtr uint32) (errno wasi.Errno) {
	dir, ok := a.opened[fd]
	if !ok || dir.fileSys == nil {
		return wasi.ErrnoInval
	}

	b, ok := ctx.Memory().Read(pathPtr, pathLen)
	if !ok {
		return wasi.ErrnoFault
	}
	path := string(b)
	f, err := dir.fileSys.OpenWASI(dirFlags, path, oFlags, fsRightsBase, fsRightsInheriting, fdFlags)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return wasi.ErrnoNoent
		default:
			return wasi.ErrnoInval
		}
	}

	newFD := a.randUnusedFD()

	a.opened[newFD] = fileEntry{
		file: f,
	}

	if !ctx.Memory().WriteUint32Le(fdPtr, newFD) {
		return wasi.ErrnoFault
	}
	return wasi.ErrnoSuccess
}

func (a *wasiAPI) fd_seek(ctx wasm.HostFunctionCallContext, fd uint32, offset uint64, whence uint32, nwrittenPtr uint32) (err wasi.Errno) {
	return wasi.ErrnoNosys // TODO: implement
}

func (a *wasiAPI) fd_write(ctx wasm.HostFunctionCallContext, fd uint32, iovsPtr uint32, iovsLen uint32, nwrittenPtr uint32) (err wasi.Errno) {
	var writer io.Writer

	switch fd {
	case 1:
		writer = a.stdout
	case 2:
		writer = a.stderr
	default:
		f, ok := a.opened[fd]
		if !ok || f.file == nil {
			return wasi.ErrnoBadf
		}
		writer = f.file
	}

	var nwritten uint32
	for i := uint32(0); i < iovsLen; i++ {
		iovPtr := iovsPtr + i*8
		offset, ok := ctx.Memory().ReadUint32Le(iovPtr)
		if !ok {
			return wasi.ErrnoFault
		}
		l, ok := ctx.Memory().ReadUint32Le(iovPtr + 4)
		if !ok {
			return wasi.ErrnoFault
		}
		b, ok := ctx.Memory().Read(offset, l)
		if !ok {
			return wasi.ErrnoFault
		}
		n, err := writer.Write(b)
		if err != nil {
			panic(err)
		}
		nwritten += uint32(n)
	}
	if !ctx.Memory().WriteUint32Le(nwrittenPtr, nwritten) {
		return wasi.ErrnoFault
	}
	return wasi.ErrnoSuccess
}

func (a *wasiAPI) fd_read(ctx wasm.HostFunctionCallContext, fd uint32, iovsPtr uint32, iovsLen uint32, nreadPtr uint32) (err wasi.Errno) {
	var reader io.Reader

	switch fd {
	case 0:
		reader = a.stdin
	default:
		f, ok := a.opened[fd]
		if !ok || f.file == nil {
			return wasi.ErrnoBadf
		}
		reader = f.file
	}

	var nread uint32
	for i := uint32(0); i < iovsLen; i++ {
		iovPtr := iovsPtr + i*8
		offset, ok := ctx.Memory().ReadUint32Le(iovPtr)
		if !ok {
			return wasi.ErrnoFault
		}
		l, ok := ctx.Memory().ReadUint32Le(iovPtr + 4)
		if !ok {
			return wasi.ErrnoFault
		}
		b, ok := ctx.Memory().Read(offset, l)
		if !ok {
			return wasi.ErrnoFault
		}
		n, err := reader.Read(b)
		nread += uint32(n)
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return wasi.ErrnoIo
		}
	}
	if !ctx.Memory().WriteUint32Le(nreadPtr, nread) {
		return wasi.ErrnoFault
	}
	return wasi.ErrnoSuccess
}

func (a *wasiAPI) fd_close(ctx wasm.HostFunctionCallContext, fd uint32) (err wasi.Errno) {
	f, ok := a.opened[fd]
	if !ok {
		return wasi.ErrnoBadf
	}

	if f.file != nil {
		f.file.Close()
	}

	delete(a.opened, fd)

	return wasi.ErrnoSuccess
}

// RandomGet implements SnapshotPreview1.RandomGet
func (a *wasiAPI) RandomGet(ctx wasm.HostFunctionCallContext, buf uint32, bufLen uint32) (errno wasi.Errno) {
	randomBytes := make([]byte, bufLen)
	err := a.randSource(randomBytes)
	if err != nil {
		// TODO: handle different errors that syscal to entropy source can return
		return wasi.ErrnoIo
	}

	if !ctx.Memory().Write(buf, randomBytes) {
		return wasi.ErrnoFault
	}

	return wasi.ErrnoSuccess
}

func proc_exit(wasm.HostFunctionCallContext, uint32) {
	// TODO: implement
}

func ValidateWASICommand(module *internalwasm.Module, moduleName string) error {
	if start, err := requireExport(module, moduleName, FunctionStart, internalwasm.ExportKindFunc); err != nil {
		return err
	} else {
		// TODO: this should be verified during decode so that errors have the correct source positions
		ft := module.TypeOfFunction(start.Index)
		if ft == nil {
			return fmt.Errorf("module[%s] function[%s] has an invalid type", moduleName, FunctionStart)
		}
		if len(ft.Params) > 0 || len(ft.Results) > 0 {
			return fmt.Errorf("module[%s] function[%s] must have an empty (nullary) signature: %s", moduleName, FunctionStart, ft.String())
		}
	}
	if _, err := requireExport(module, moduleName, FunctionInitialize, internalwasm.ExportKindFunc); err == nil {
		return fmt.Errorf("module[%s] must not export func[%s]", moduleName, FunctionInitialize)
	}
	if _, err := requireExport(module, moduleName, "memory", internalwasm.ExportKindMemory); err != nil {
		return err
	}
	// TODO: the spec also requires export of "__indirect_function_table", but we aren't enforcing it, and doing so
	// would break existing users of TinyGo who aren't exporting that. We could possibly scan to see if it is every used.
	return nil
}

func requireExport(module *internalwasm.Module, moduleName string, exportName string, kind internalwasm.ExportKind) (*internalwasm.Export, error) {
	exp, ok := module.ExportSection[exportName]
	if !ok || exp.Kind != kind {
		return nil, fmt.Errorf("module[%s] does not export %s[%s]", moduleName, internalwasm.ExportKindName(kind), exportName)
	}
	return exp, nil
}
