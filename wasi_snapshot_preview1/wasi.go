// Package wasi_snapshot_preview1 contains Go-defined functions to access
// system calls, such as opening a file, similar to Go's x/sys package. These
// are accessible from WebAssembly-defined functions via importing ModuleName.
// All WASI functions return a single Errno result: ErrnoSuccess on success.
//
// Ex. Call Instantiate before instantiating any wasm binary that imports
// "wasi_snapshot_preview1", Otherwise, it will error due to missing imports.
//	ctx := context.Background()
//	r := wazero.NewRuntime()
//	defer r.Close(ctx) // This closes everything this Runtime created.
//
//	_, _ = Instantiate(ctx, r)
//	mod, _ := r.InstantiateModuleFromBinary(ctx, wasm)
//
// See https://github.com/WebAssembly/WASI
package wasi_snapshot_preview1

import (
	"context"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// ModuleName is the module name WASI functions are exported into.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md
const ModuleName = "wasi_snapshot_preview1"
const i32, i64 = wasm.ValueTypeI32, wasm.ValueTypeI64

// Instantiate instantiates the ModuleName module into the runtime default
// namespace.
//
// Notes
//
//	* Closing the wazero.Runtime has the same effect as closing the result.
//	* To instantiate into another wazero.Namespace, use NewBuilder instead.
func Instantiate(ctx context.Context, r wazero.Runtime) (api.Closer, error) {
	return NewBuilder(r).Instantiate(ctx, r)
}

// Builder configures the ModuleName module for later use via Compile or Instantiate.
type Builder interface {

	// Compile compiles the ModuleName module that can instantiated in any
	// namespace (wazero.Namespace).
	//
	// Note: This has the same effect as the same function on wazero.ModuleBuilder.
	Compile(context.Context, wazero.CompileConfig) (wazero.CompiledModule, error)

	// Instantiate instantiates the ModuleName module into the given namespace.
	//
	// Note: This has the same effect as the same function on wazero.ModuleBuilder.
	Instantiate(context.Context, wazero.Namespace) (api.Closer, error)
}

// NewBuilder returns a new Builder.
func NewBuilder(r wazero.Runtime) Builder {
	return &builder{r}
}

type builder struct{ r wazero.Runtime }

// moduleBuilder returns a new wazero.ModuleBuilder for ModuleName
func (b *builder) moduleBuilder() wazero.ModuleBuilder {
	ret := b.r.NewModuleBuilder(ModuleName)
	exportFunctions(ret)
	return ret
}

// Compile implements Builder.Compile
func (b *builder) Compile(ctx context.Context, config wazero.CompileConfig) (wazero.CompiledModule, error) {
	return b.moduleBuilder().Compile(ctx, config)
}

// Instantiate implements Builder.Instantiate
func (b *builder) Instantiate(ctx context.Context, ns wazero.Namespace) (api.Closer, error) {
	return b.moduleBuilder().Instantiate(ctx, ns)
}

// ## Translation notes
// ### String
// WebAssembly 1.0 has no string type, so any string input parameter expands to two uint32 parameters: offset
// and length.
//
// ### iovec_array
// `iovec_array` is encoded as two uin32le values (i32): offset and count.
//
// ### Result
// Each result besides Errno is always an uint32 parameter. WebAssembly 1.0 can have up to one result,
// which is already used by Errno. This forces other results to be parameters. A result parameter is a memory
// offset to write the result to. As memory offsets are uint32, each parameter representing a result is uint32.
//
// ### Errno
// The WASI specification is sometimes ambiguous resulting in some runtimes interpreting the same function ways.
// Errno mappings are not defined in WASI, yet, so these mappings are best efforts by maintainers. When in doubt
// about portability, first look at /RATIONALE.md and if needed an issue on
// https://github.com/WebAssembly/WASI/issues
//
// ## Memory
// In WebAssembly 1.0 (20191205), there may be up to one Memory per store, which means api.Memory is always the
// wasm.Store Memories index zero: `store.Memories[0].Buffer`
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md
// See https://github.com/WebAssembly/WASI/issues/215
// See https://wwa.w3.org/TR/2019/REC-wasm-core-1-20191205/#memory-instances%E2%91%A0.

// exportFunctions adds all go functions that implement wasi.
// These should be exported in the module named ModuleName.
func exportFunctions(builder wazero.ModuleBuilder) {
	// Note:se are ordered per spec for consistency even if the resulting map can't guarantee that.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#functions
	builder.ExportFunction(functionArgsGet, argsGet,
		functionArgsGet, "argv", "argv_buf")
	builder.ExportFunction(functionArgsSizesGet, argsSizesGet,
		functionArgsSizesGet, "result.argc", "result.argv_buf_size")
	builder.ExportFunction(functionEnvironGet, environGet,
		functionEnvironGet, "environ", "environ_buf")
	builder.ExportFunction(functionEnvironSizesGet, environSizesGet,
		functionEnvironSizesGet, "result.environc", "result.environBufSize")
	builder.ExportFunction(functionClockResGet, clockResGet,
		functionClockResGet, "id", "result.resolution")
	builder.ExportFunction(functionClockTimeGet, clockTimeGet,
		functionClockTimeGet, "id", "precision", "result.timestamp")
	builder.ExportFunction(functionFdAdvise, fdAdvise,
		functionFdAdvise, "fd", "offset", "len", "result.advice")
	builder.ExportFunction(functionFdAllocate, fdAllocate,
		functionFdAllocate, "fd", "offset", "len")
	builder.ExportFunction(functionFdClose, fdClose,
		functionFdClose, "fd")
	builder.ExportFunction(functionFdDatasync, fdDatasync,
		functionFdDatasync, "fd")
	builder.ExportFunction(functionFdFdstatGet, fdFdstatGet,
		functionFdFdstatGet, "fd", "result.stat")
	builder.ExportFunction(functionFdFdstatSetFlags, fdFdstatSetFlags,
		functionFdFdstatSetFlags, "fd", "flags")
	builder.ExportFunction(functionFdFdstatSetRights, fdFdstatSetRights,
		functionFdFdstatSetRights, "fd", "fs_rights_base", "fs_rights_inheriting")
	builder.ExportFunction(functionFdFilestatGet, fdFilestatGet,
		functionFdFilestatGet, "fd", "result.buf")
	builder.ExportFunction(functionFdFilestatSetSize, fdFilestatSetSize,
		functionFdFilestatSetSize, "fd", "size")
	builder.ExportFunction(functionFdFilestatSetTimes, fdFilestatSetTimes,
		functionFdFilestatSetTimes, "fd", "atim", "mtim", "fst_flags")
	builder.ExportFunction(functionFdPread, fdPread,
		functionFdPread, "fd", "iovs", "iovs_len", "offset", "result.nread")
	builder.ExportFunction(functionFdPrestatGet, fdPrestatGet,
		functionFdPrestatGet, "fd", "result.prestat")
	builder.ExportFunction(functionFdPrestatDirName, fdPrestatDirName,
		functionFdPrestatDirName, "fd", "path", "path_len")
	builder.ExportFunction(functionFdPwrite, fdPwrite,
		functionFdPwrite, "fd", "iovs", "iovs_len", "offset", "result.nwritten")
	builder.ExportFunction(functionFdRead, fdRead,
		functionFdRead, "fd", "iovs", "iovs_len", "result.size")
	builder.ExportFunction(functionFdReaddir, fdReaddir,
		functionFdReaddir, "fd", "buf", "buf_len", "cookie", "result.bufused")
	builder.ExportFunction(functionFdRenumber, fdRenumber,
		functionFdRenumber, "fd", "to")
	builder.ExportFunction(functionFdSeek, fdSeek,
		functionFdSeek, "fd", "offset", "whence", "result.newoffset")
	builder.ExportFunction(functionFdSync, fdSync,
		functionFdSync, "fd")
	builder.ExportFunction(functionFdTell, fdTell,
		functionFdTell, "fd", "result.offset")
	builder.ExportFunction(functionFdWrite, fdWrite,
		functionFdWrite, "fd", "iovs", "iovs_len", "result.size")
	builder.ExportFunction(functionPathCreateDirectory, pathCreateDirectory,
		functionPathCreateDirectory, "fd", "path", "path_len")
	builder.ExportFunction(functionPathFilestatGet, pathFilestatGet,
		functionPathFilestatGet, "fd", "flags", "path", "path_len", "result.buf")
	builder.ExportFunction(functionPathFilestatSetTimes, pathFilestatSetTimes,
		functionPathFilestatSetTimes, "fd", "flags", "path", "path_len", "atim", "mtim", "fst_flags")
	builder.ExportFunction(functionPathLink, pathLink,
		functionPathLink, "old_fd", "old_flags", "old_path", "old_path_len", "new_fd", "new_path", "new_path_len")
	builder.ExportFunction(functionPathOpen, pathOpen,
		functionPathOpen, "fd", "dirflags", "path", "path_len", "oflags", "fs_rights_base", "fs_rights_inheriting", "fdflags", "result.opened_fd")
	builder.ExportFunction(functionPathReadlink, pathReadlink,
		functionPathReadlink, "fd", "path", "path_len", "buf", "buf_len", "result.bufused")
	builder.ExportFunction(functionPathRemoveDirectory, pathRemoveDirectory,
		functionPathRemoveDirectory, "fd", "path", "path_len")
	builder.ExportFunction(functionPathRename, pathRename,
		functionPathRename, "fd", "old_path", "old_path_len", "new_fd", "new_path", "new_path_len")
	builder.ExportFunction(functionPathSymlink, pathSymlink,
		functionPathSymlink, "old_path", "old_path_len", "fd", "new_path", "new_path_len")
	builder.ExportFunction(functionPathUnlinkFile, pathUnlinkFile,
		functionPathUnlinkFile, "fd", "path", "path_len")
	builder.ExportFunction(functionPollOneoff, pollOneoff,
		functionPollOneoff, "in", "out", "nsubscriptions", "result.nevents")
	builder.ExportFunction(functionProcExit, procExit,
		functionProcExit, "rval")
	builder.ExportFunction(functionProcRaise, procRaise,
		functionProcRaise, "sig")
	builder.ExportFunction(functionSchedYield, schedYield,
		functionSchedYield)
	builder.ExportFunction(functionRandomGet, randomGet,
		functionRandomGet, "buf", "buf_len")
	builder.ExportFunction(functionSockRecv, sockRecv,
		functionSockRecv, "fd", "ri_data", "ri_data_count", "ri_flags", "result.ro_datalen", "result.ro_flags")
	builder.ExportFunction(functionSockSend, sockSend,
		functionSockSend, "fd", "si_data", "si_data_count", "si_flags", "result.so_datalen")
	builder.ExportFunction(functionSockShutdown, sockShutdown,
		functionSockShutdown, "fd", "how")
}

func writeOffsetsAndNullTerminatedValues(ctx context.Context, mem api.Memory, values []string, offsets, bytes uint32) Errno {
	for _, value := range values {
		// Write current offset and advance it.
		if !mem.WriteUint32Le(ctx, offsets, bytes) {
			return ErrnoFault
		}
		offsets += 4 // size of uint32

		// Write the next value to memory with a NUL terminator
		if !mem.Write(ctx, bytes, []byte(value)) {
			return ErrnoFault
		}
		bytes += uint32(len(value))
		if !mem.WriteByte(ctx, bytes, 0) {
			return ErrnoFault
		}
		bytes++
	}

	return ErrnoSuccess
}

// stubFunction returns a function for the given params which returns ErrnoNosys.
func stubFunction(params ...wasm.ValueType) *wasm.Func {
	return &wasm.Func{
		Type: &wasm.FunctionType{
			Params:            params,
			Results:           []wasm.ValueType{wasm.ValueTypeI32},
			ParamNumInUint64:  len(params),
			ResultNumInUint64: 1,
		},
		Code: &wasm.Code{Body: []byte{wasm.OpcodeI32Const, byte(ErrnoNosys), wasm.OpcodeEnd}},
	}
}
