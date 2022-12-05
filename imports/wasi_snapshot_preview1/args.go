package wasi_snapshot_preview1

import (
	"context"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const (
	argsGetName      = "args_get"
	argsSizesGetName = "args_sizes_get"
)

// argsGet is the WASI function named argsGetName that reads command-line
// argument data.
//
// # Parameters
//
//   - argv: offset to begin writing argument offsets in uint32 little-endian
//     encoding to api.Memory
//   - argsSizesGet result argc * 4 bytes are written to this offset
//   - argvBuf: offset to write the null terminated arguments to api.Memory
//   - argsSizesGet result argv_len bytes are written to this offset
//
// Result (Errno)
//
// The return value is ErrnoSuccess except the following error conditions:
//   - ErrnoFault: there is not enough memory to write results
//
// For example, if argsSizesGet wrote argc=2 and argvLen=5 for arguments:
// "a" and "bc" parameters argv=7 and argvBuf=1, this function writes the below
// to api.Memory:
//
//	                   argvLen          uint32le    uint32le
//	            +----------------+     +--------+  +--------+
//	            |                |     |        |  |        |
//	 []byte{?, 'a', 0, 'b', 'c', 0, ?, 1, 0, 0, 0, 3, 0, 0, 0, ?}
//	argvBuf --^                      ^           ^
//	                          argv --|           |
//	        offset that begins "a" --+           |
//	                   offset that begins "bc" --+
//
// See argsSizesGet
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#args_get
// See https://en.wikipedia.org/wiki/Null-terminated_string
var argsGet = newHostFunc(argsGetName, argsGetFn, []api.ValueType{i32, i32}, "argv", "argv_buf")

func argsGetFn(ctx context.Context, mod api.Module, params []uint64) Errno {
	sysCtx := mod.(*wasm.CallContext).Sys
	argv, argvBuf := uint32(params[0]), uint32(params[1])
	return writeOffsetsAndNullTerminatedValues(ctx, mod.Memory(), sysCtx.Args(), argv, argvBuf, sysCtx.ArgsSize())
}

// argsSizesGet is the WASI function named argsSizesGetName that reads
// command-line argument sizes.
//
// # Parameters
//
//   - resultArgc: offset to write the argument count to api.Memory
//   - resultArgvLen: offset to write the null-terminated argument length to
//     api.Memory
//
// Result (Errno)
//
// The return value is ErrnoSuccess except the following error conditions:
//   - ErrnoFault: there is not enough memory to write results
//
// For example, if args are "a", "bc" and parameters resultArgc=1 and
// resultArgvLen=6, this function writes the below to api.Memory:
//
//	                uint32le       uint32le
//	               +--------+     +--------+
//	               |        |     |        |
//	     []byte{?, 2, 0, 0, 0, ?, 5, 0, 0, 0, ?}
//	  resultArgc --^              ^
//	      2 args --+              |
//	              resultArgvLen --|
//	len([]byte{'a',0,'b',c',0}) --+
//
// See argsGet
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#args_sizes_get
// See https://en.wikipedia.org/wiki/Null-terminated_string
var argsSizesGet = proxyResultParams(&wasm.HostFunc{
	Name:        "argsSizesGet",
	ResultTypes: []api.ValueType{i32, i32, i32},
	ResultNames: []string{"argc", "argv_len", "errno"},
	Code:        &wasm.Code{IsHostFunction: true, GoFunc: u32u32ResultParam(argsSizesGetFn)},
}, argsSizesGetName)

func argsSizesGetFn(_ context.Context, mod api.Module, _ []uint64) (argc, argvLen uint32, errno Errno) {
	sysCtx := mod.(*wasm.CallContext).Sys
	argc = uint32(len(sysCtx.Args()))
	argvLen = sysCtx.ArgsSize()
	errno = ErrnoSuccess
	return
}
