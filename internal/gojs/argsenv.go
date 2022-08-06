package gojs

import (
	"context"
	"errors"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// Constants about memory layout. See REFERENCE.md
const (
	endOfPageZero     = uint32(4096)                      // runtime.minLegalPointer
	maxArgsAndEnviron = uint32(8192)                      // ld.wasmMinDataAddr - runtime.minLegalPointer
	wasmMinDataAddr   = endOfPageZero + maxArgsAndEnviron // ld.wasmMinDataAddr
)

// WriteArgsAndEnviron writes arguments and environment variables to memory, so
// they can be read by main, Go compiles as the function export "run".
func WriteArgsAndEnviron(ctx context.Context, mod api.Module) (argc, argv uint32, err error) {
	mem := mod.Memory()
	sysCtx := mod.(*wasm.CallContext).Sys
	args := sysCtx.Args()
	environ := sysCtx.Environ()

	argc = uint32(len(args))
	offset := endOfPageZero

	strPtr := func(val, field string, i int) (ptr uint32) {
		// TODO: return err and format "%s[%d], field, i"
		ptr = offset
		mustWrite(ctx, mem, field, offset, append([]byte(val), 0))
		offset += uint32(len(val) + 1)
		if pad := offset % 8; pad != 0 {
			offset += 8 - pad
		}
		return
	}
	argvPtrs := make([]uint32, 0, len(args)+1+len(environ)+1)
	for i, arg := range args {
		argvPtrs = append(argvPtrs, strPtr(arg, "args", i))
	}
	argvPtrs = append(argvPtrs, 0)

	for i, env := range environ {
		argvPtrs = append(argvPtrs, strPtr(env, "env", i))
	}
	argvPtrs = append(argvPtrs, 0)

	argv = offset
	for _, ptr := range argvPtrs {
		// TODO: return err and format "argvPtrs[%d], i"
		mustWriteUint64Le(ctx, mem, "argvPtrs[i]", offset, uint64(ptr))
		offset += 8
	}

	if offset >= wasmMinDataAddr {
		err = errors.New("total length of command line and environment variables exceeds limit")
	}
	return
}
