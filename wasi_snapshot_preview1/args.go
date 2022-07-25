package wasi_snapshot_preview1

import (
	"context"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const (
	functionArgsGet      = "args_get"
	functionArgsSizesGet = "args_sizes_get"
)

// argsGet is the WASI function named functionArgsGet that reads command-line
// argument data.
//
// # Parameters
//
//   - argv: offset to begin writing argument offsets in uint32 little-endian
//     encoding to api.Memory
//   - argsSizesGet result argc * 4 bytes are written to this offset
//   - argvBuf: offset to write the null terminated arguments to api.Memory
//   - argsSizesGet result argv_buf_size bytes are written to this offset
//
// Result (Errno)
//
// The return value is ErrnoSuccess except the following error conditions:
//   - ErrnoFault: there is not enough memory to write results
//
// For example, if argsSizesGet wrote argc=2 and argvBufSize=5 for arguments:
// "a" and "bc" parameters argv=7 and argvBuf=1, this function writes the below
// to api.Memory:
//
//	   argvBufSize          uint32le    uint32le
//	+----------------+     +--------+  +--------+
//	|                |     |        |  |        |
//
// []byte{?, 'a', 0, 'b', 'c', 0, ?, 1, 0, 0, 0, 3, 0, 0, 0, ?}
//
//	argvBuf --^                      ^           ^
//	                          argv --|           |
//	        offset that begins "a" --+           |
//	                   offset that begins "bc" --+
//
// See argsSizesGet
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#args_get
// See https://en.wikipedia.org/wiki/Null-terminated_string
var argsGet = wasm.NewGoFunc(
	functionArgsGet, functionArgsGet,
	[]string{"argv", "argv_buf"},
	func(ctx context.Context, mod api.Module, argv, argvBuf uint32) Errno {
		sysCtx := mod.(*wasm.CallContext).Sys
		return writeOffsetsAndNullTerminatedValues(ctx, mod.Memory(), sysCtx.Args(), argv, argvBuf)
	},
)

// argsSizesGet is the WASI function named functionArgsSizesGet that reads
// command-line argument sizes.
//
// # Parameters
//
//   - resultArgc: offset to write the argument count to api.Memory
//   - resultArgvBufSize: offset to write the null-terminated argument length to
//     api.Memory
//
// Result (Errno)
//
// The return value is ErrnoSuccess except the following error conditions:
//   - ErrnoFault: there is not enough memory to write results
//
// For example, if args are "a", "bc" and parameters resultArgc=1 and
// resultArgvBufSize=6, this function writes the below to api.Memory:
//
//	                uint32le       uint32le
//	               +--------+     +--------+
//	               |        |     |        |
//	     []byte{?, 2, 0, 0, 0, ?, 5, 0, 0, 0, ?}
//	  resultArgc --^              ^
//	      2 args --+              |
//	          resultArgvBufSize --|
//	len([]byte{'a',0,'b',c',0}) --+
//
// See argsGet
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#args_sizes_get
// See https://en.wikipedia.org/wiki/Null-terminated_string
var argsSizesGet = wasm.NewGoFunc(
	functionArgsSizesGet, functionArgsSizesGet,
	[]string{"result.argc", "result.argv_buf_size"},
	func(ctx context.Context, mod api.Module, resultArgc, resultArgvBufSize uint32) Errno {
		sysCtx := mod.(*wasm.CallContext).Sys
		mem := mod.Memory()

		if !mem.WriteUint32Le(ctx, resultArgc, uint32(len(sysCtx.Args()))) {
			return ErrnoFault
		}
		if !mem.WriteUint32Le(ctx, resultArgvBufSize, sysCtx.ArgsSize()) {
			return ErrnoFault
		}
		return ErrnoSuccess
	},
)
