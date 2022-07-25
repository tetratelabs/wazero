package wasi_snapshot_preview1

import (
	"context"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const (
	functionEnvironGet      = "environ_get"
	functionEnvironSizesGet = "environ_sizes_get"
)

// environGet is the WASI function named functionEnvironGet that reads
// environment variables.
//
// # Parameters
//
//   - environ: offset to begin writing environment offsets in uint32
//     little-endian encoding to api.Memory
//   - environSizesGet result environc * 4 bytes are written to this offset
//   - environBuf: offset to write the null-terminated variables to api.Memory
//   - the format is like os.Environ: null-terminated "key=val" entries
//   - environSizesGet result environBufSize bytes are written to this offset
//
// Result (Errno)
//
// The return value is ErrnoSuccess except the following error conditions:
//   - ErrnoFault: there is not enough memory to write results
//
// For example, if environSizesGet wrote environc=2 and environBufSize=9 for
// environment variables: "a=b", "b=cd" and parameters environ=11 and
// environBuf=1, this function writes the below to api.Memory:
//
//	                        environBufSize                 uint32le    uint32le
//	           +------------------------------------+     +--------+  +--------+
//	           |                                    |     |        |  |        |
//	[]byte{?, 'a', '=', 'b', 0, 'b', '=', 'c', 'd', 0, ?, 1, 0, 0, 0, 5, 0, 0, 0, ?}
//
// environBuf --^                                          ^           ^
//
//	environ offset for "a=b" --+           |
//	           environ offset for "b=cd" --+
//
// See environSizesGet
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#environ_get
// See https://en.wikipedia.org/wiki/Null-terminated_string
var environGet = wasm.NewGoFunc(
	functionEnvironGet, functionEnvironGet,
	[]string{"environ", "environ_buf"},
	func(ctx context.Context, mod api.Module, environ uint32, environBuf uint32) Errno {
		sysCtx := mod.(*wasm.CallContext).Sys
		return writeOffsetsAndNullTerminatedValues(ctx, mod.Memory(), sysCtx.Environ(), environ, environBuf)
	},
)

// environSizesGet is the WASI function named functionEnvironSizesGet that
// reads environment variable sizes.
//
// # Parameters
//
//   - resultEnvironc: offset to write the count of environment variables to
//     api.Memory
//   - resultEnvironBufSize: offset to write the null-terminated environment
//     variable length to api.Memory
//
// Result (Errno)
//
// The return value is ErrnoSuccess except the following error conditions:
//   - ErrnoFault: there is not enough memory to write results
//
// For example, if environ are "a=b","b=cd" and parameters resultEnvironc=1 and
// resultEnvironBufSize=6, this function writes the below to api.Memory:
//
//	           uint32le       uint32le
//	          +--------+     +--------+
//	          |        |     |        |
//	[]byte{?, 2, 0, 0, 0, ?, 9, 0, 0, 0, ?}
//
// resultEnvironc --^              ^
//
//	2 variables --+              |
//	      resultEnvironBufSize --|
//	len([]byte{'a','=','b',0,    |
//	       'b','=','c','d',0}) --+
//
// See environGet
// https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#environ_sizes_get
// and https://en.wikipedia.org/wiki/Null-terminated_string
var environSizesGet = wasm.NewGoFunc(
	functionEnvironSizesGet, functionEnvironSizesGet,
	[]string{"result.environc", "result.environBufSize"},
	func(ctx context.Context, mod api.Module, resultEnvironc uint32, resultEnvironBufSize uint32) Errno {
		sysCtx := mod.(*wasm.CallContext).Sys
		mem := mod.Memory()

		if !mem.WriteUint32Le(ctx, resultEnvironc, uint32(len(sysCtx.Environ()))) {
			return ErrnoFault
		}
		if !mem.WriteUint32Le(ctx, resultEnvironBufSize, sysCtx.EnvironSize()) {
			return ErrnoFault
		}

		return ErrnoSuccess
	},
)
