// Package wasi_snapshot_preview1 contains Go-defined functions to access
// system calls, such as opening a file, similar to Go's x/sys package. These
// are accessible from WebAssembly-defined functions via importing ModuleName.
// All WASI functions return a single Errno result: ErrnoSuccess on success.
//
// Ex. Call Instantiate before instantiating any wasm binary that imports
// "wasi_snapshot_preview1", Otherwise, it will error due to missing imports.
//
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
// # Notes
//
//   - Closing the wazero.Runtime has the same effect as closing the result.
//   - To instantiate into another wazero.Namespace, use NewBuilder instead.
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
	builder.ExportFunction(argsGet.Name, argsGet)
	builder.ExportFunction(argsSizesGet.Name, argsSizesGet)
	builder.ExportFunction(environGet.Name, environGet)
	builder.ExportFunction(environSizesGet.Name, environSizesGet)
	builder.ExportFunction(clockResGet.Name, clockResGet)
	builder.ExportFunction(clockTimeGet.Name, clockTimeGet)
	builder.ExportFunction(fdAdvise.Name, fdAdvise)
	builder.ExportFunction(fdAllocate.Name, fdAllocate)
	builder.ExportFunction(fdClose.Name, fdClose)
	builder.ExportFunction(fdDatasync.Name, fdDatasync)
	builder.ExportFunction(fdFdstatGet.Name, fdFdstatGet)
	builder.ExportFunction(fdFdstatSetFlags.Name, fdFdstatSetFlags)
	builder.ExportFunction(fdFdstatSetRights.Name, fdFdstatSetRights)
	builder.ExportFunction(fdFilestatGet.Name, fdFilestatGet)
	builder.ExportFunction(fdFilestatSetSize.Name, fdFilestatSetSize)
	builder.ExportFunction(fdFilestatSetTimes.Name, fdFilestatSetTimes)
	builder.ExportFunction(fdPread.Name, fdPread)
	builder.ExportFunction(fdPrestatGet.Name, fdPrestatGet)
	builder.ExportFunction(fdPrestatDirName.Name, fdPrestatDirName)
	builder.ExportFunction(fdPwrite.Name, fdPwrite)
	builder.ExportFunction(fdRead.Name, fdRead)
	builder.ExportFunction(fdReaddir.Name, fdReaddir)
	builder.ExportFunction(fdRenumber.Name, fdRenumber)
	builder.ExportFunction(fdSeek.Name, fdSeek)
	builder.ExportFunction(fdSync.Name, fdSync)
	builder.ExportFunction(fdTell.Name, fdTell)
	builder.ExportFunction(fdWrite.Name, fdWrite)
	builder.ExportFunction(pathCreateDirectory.Name, pathCreateDirectory)
	builder.ExportFunction(pathFilestatGet.Name, pathFilestatGet)
	builder.ExportFunction(pathFilestatSetTimes.Name, pathFilestatSetTimes)
	builder.ExportFunction(pathLink.Name, pathLink)
	builder.ExportFunction(pathOpen.Name, pathOpen)
	builder.ExportFunction(pathReadlink.Name, pathReadlink)
	builder.ExportFunction(pathRemoveDirectory.Name, pathRemoveDirectory)
	builder.ExportFunction(pathRename.Name, pathRename)
	builder.ExportFunction(pathSymlink.Name, pathSymlink)
	builder.ExportFunction(pathUnlinkFile.Name, pathUnlinkFile)
	builder.ExportFunction(pollOneoff.Name, pollOneoff)
	builder.ExportFunction(procExit.Name, procExit)
	builder.ExportFunction(procRaise.Name, procRaise)
	builder.ExportFunction(schedYield.Name, schedYield)
	builder.ExportFunction(randomGet.Name, randomGet)
	builder.ExportFunction(sockRecv.Name, sockRecv)
	builder.ExportFunction(sockSend.Name, sockSend)
	builder.ExportFunction(sockShutdown.Name, sockShutdown)
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

// stubFunction stubs for GrainLang per #271.
func stubFunction(name string, paramTypes []wasm.ValueType, paramNames []string) *wasm.HostFunc {
	return &wasm.HostFunc{
		Name:        name,
		ExportNames: []string{name},
		ParamTypes:  paramTypes,
		ParamNames:  paramNames,
		ResultTypes: []wasm.ValueType{i32},
		Code: &wasm.Code{
			IsHostFunction: true,
			Body:           []byte{wasm.OpcodeI32Const, byte(ErrnoNosys), wasm.OpcodeEnd},
		},
	}
}
