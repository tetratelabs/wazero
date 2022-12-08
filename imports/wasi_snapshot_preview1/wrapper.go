package wasi_snapshot_preview1

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

type u32ResultParam func(ctx context.Context, mod api.Module, stack []uint64) (uint32, Errno)

// Call implements the same method as documented on api.GoModuleFunc.
func (f u32ResultParam) Call(ctx context.Context, mod api.Module, stack []uint64) {
	r1, errno := f(ctx, mod, stack)
	stack[0] = uint64(r1)
	stack[1] = uint64(errno)
}

type i64ResultParam func(ctx context.Context, mod api.Module, stack []uint64) (int64, Errno)

// Call implements the same method as documented on api.GoModuleFunc.
func (f i64ResultParam) Call(ctx context.Context, mod api.Module, stack []uint64) {
	r1, errno := f(ctx, mod, stack)
	stack[0] = uint64(r1)
	stack[1] = uint64(errno)
}

type u64ResultParam func(ctx context.Context, mod api.Module, stack []uint64) (uint64, Errno)

// Call implements the same method as documented on api.GoModuleFunc.
func (f u64ResultParam) Call(ctx context.Context, mod api.Module, stack []uint64) {
	r1, errno := f(ctx, mod, stack)
	stack[0] = r1
	stack[1] = uint64(errno)
}

type u32u32ResultParam func(ctx context.Context, mod api.Module, stack []uint64) (r1, r2 uint32, errno Errno)

// Call implements the same method as documented on api.GoModuleFunc.
func (f u32u32ResultParam) Call(ctx context.Context, mod api.Module, stack []uint64) {
	r1, r2, errno := f(ctx, mod, stack)
	stack[0] = uint64(r1)
	stack[1] = uint64(r2)
	stack[2] = uint64(errno)
}

// memBytes adds an i32 value of memory[0]'s size in bytes to the stack
var memBytes = append(append([]byte{wasm.OpcodeMemorySize, 0, wasm.OpcodeI32Const}, leb128.EncodeInt32(int32(wasm.MemoryPageSize))...), wasm.OpcodeI32Mul)

const (
	nilType = 0x40
	success = byte(ErrnoSuccess)
	fault   = byte(ErrnoFault)
	// fakeFuncIdx is a placeholder as it is replaced later.
	fakeFuncIdx = byte(0)
)

// proxyResultParams defines a function in wasm that has out parameters for each
// result in the goFunc that isn't Errno. An out parameter is an i32 of the
// memory offset to write the result length.
//
// # Example
//
// For example, if goFunc has this signature:
//
//	(func $argsSizesGet
//	  (result (; argc ;) i32)
//	  (result (; argv_len ;) i32)
//	  (result (; errno ;) i32) ...)
//
// This would write the following signature:
//
//	(func (export "args_sizes_get")
//	  (param $result.argc_len i32)
//	  (param $result.argv_len i32)
//	  (result (; errno ;) i32) ...)
//
// "args_sizes_get" would verify the memory offsets of the "result.*"
// parameters, then call "argsSizesGet". The first two results would be written
// to the corresponding "result.*" parameters if the errno is ErrnoSuccess. In
// any case the errno is propagated to the caller of "args_sizes_get".
//
// # Why does this exist
//
// Proxying allows less code in the go-defined function, and also allows the
// natural results to be visible to logging or otherwise interceptors. This
// helps in troubleshooting, particularly as some WASI results are inputs to
// subsequent functions, and when invalid can cause errors difficult to
// diagnose. For example, the interceptor could see an api.ValueTypeI32 result
// representing bufused and write it to the log. Without this, the same
// interceptor would need access to wasm memory to read the value, and it would
// be generally impossible get insight without a debugger.
func proxyResultParams(goFunc *wasm.HostFunc, exportName string) *wasm.ProxyFunc {
	// result params are those except errno
	outLen := len(goFunc.ResultTypes) - 1
	outTypes := goFunc.ResultTypes[:outLen]

	// Conventionally, we name the out parameters result.X
	outParamNames := make([]string, outLen)
	for i := 0; i < outLen; i++ {
		outParamNames[i] = "result." + goFunc.ResultNames[i]
	}

	// outParams are i32, even if the corresponding outParamLengths are not,
	// because memory offsets are i32.
	outParamTypes := make([]api.ValueType, outLen)
	for i := 0; i < outLen; i++ {
		outParamTypes[i] = i32
	}
	outParamIdx := len(goFunc.ParamTypes)

	proxy := &wasm.HostFunc{
		ExportNames: []string{exportName},
		Name:        exportName,
		ParamTypes:  append(goFunc.ParamTypes, outParamTypes...),
		ParamNames:  append(goFunc.ParamNames, outParamNames...),
		ResultTypes: []api.ValueType{i32},
		ResultNames: []string{"errno"},
		Code:        &wasm.Code{IsHostFunction: true, LocalTypes: []api.ValueType{i32}},
	}

	// Determine the index of a temporary local used for i32 manipulation. This
	// will be directly after the parameters.
	localValIdxI32 := byte(len(proxy.ParamTypes))

	// Only allocate an i64 local if an out type is i64
	localI64Temp := localValIdxI32 + 1
	for _, t := range outTypes {
		if t == i64 {
			proxy.Code.LocalTypes = append(proxy.Code.LocalTypes, i64)
			break
		}
	}

	// Get the current memory size in bytes, to validate parameters.
	body := memBytes                                         // stack: [memSizeBytes]
	body = append(body, wasm.OpcodeLocalTee, localValIdxI32) // stack: [memSizeBytes]

	// Write code to verify out parameters are in range to write values.
	// Notably, this validates prior to calling the host function.
	body = append(body, compileMustValidMemOffset(outParamIdx, outTypes[0])...)
	for i := 1; i < outLen; i++ {
		body = append(body, wasm.OpcodeLocalGet, localValIdxI32) // stack: [memSizeBytes]
		body = append(body, compileMustValidMemOffset(outParamIdx+i, outTypes[i])...)
	}

	// Now, it is safe to call the go function
	for i := range goFunc.ParamTypes {
		body = append(body, wasm.OpcodeLocalGet, byte(i)) // stack: [p1, p2...]
	}
	body = append(body, wasm.OpcodeCall, fakeFuncIdx) // stack: [r1, r2... errno]
	// Capture the position in the wasm ProxyFunc rewrites with a real index.
	fakeFuncIdxPos := len(body) - 1

	// On failure, return the errno
	body = append(body, compileMustErrnoSuccess(localValIdxI32)...) // stack: [r1, r2...]

	// On success, results to write to memory are in backwards order.
	for i := outLen - 1; i >= 0; i-- {
		outType := outTypes[i]
		localValIdx := localValIdxI32
		switch outType {
		case i32:
		case i64:
			localValIdx = localI64Temp
		default:
			panic(fmt.Errorf("TODO: unsupported outType: %v", outType))
		}
		localOffsetIdx := byte(outParamIdx + i)
		body = append(body, compileStore(localOffsetIdx, localValIdx, outType)...)
	}

	// Finally, add the success error code to the stack.
	body = append(body,
		wasm.OpcodeI32Const, success, // stack: [success]
		wasm.OpcodeEnd,
	)

	// Assign the wasm generated as the proxy's body
	proxy.Code.Body = body
	return &wasm.ProxyFunc{Proxy: proxy, Proxied: goFunc, CallBodyPos: fakeFuncIdxPos}
}

// compileMustErrnoSuccess returns the stack top value if it isn't
// ErrnoSuccess.
func compileMustErrnoSuccess(localI32Temp byte) []byte {
	// copy the errno to a local, so we can return it later if needed
	return []byte{
		wasm.OpcodeLocalTee, localI32Temp, // stack: [errno]
		wasm.OpcodeI32Const, success, // stack: [errno, success]

		// If the result wasn't success, return errno.
		wasm.OpcodeI32Ne, wasm.OpcodeIf, nilType, // stack: []
		wasm.OpcodeLocalGet, localI32Temp, // stack: [errno]
		wasm.OpcodeReturn, wasm.OpcodeEnd, // stack: []
	}
}

// compileMustValidMemOffset returns ErrnoFault if params[paramIdx]+4
// exceeds available memory (without growing). The stack top value must be
// memories[0] size in bytes.
func compileMustValidMemOffset(paramIdx int, outType api.ValueType) []byte {
	byteLength := 4
	switch outType {
	case i32:
	case i64:
		byteLength = 8
	default:
		panic(fmt.Errorf("TODO: unsupported outType: %v", outType))
	}
	return []byte{
		wasm.OpcodeI32Const, byte(byteLength), wasm.OpcodeI32Sub, // stack: [memBytes-byteLength]
		wasm.OpcodeLocalGet, byte(paramIdx), // stack: [memBytes-byteLength, $0]
		wasm.OpcodeI32LtU, wasm.OpcodeIf, nilType, // stack: []
		wasm.OpcodeI32Const, fault, // stack: [efault]
		wasm.OpcodeReturn, wasm.OpcodeEnd, // stack: []
	}
}

// compileStore stores stack top stack value to the memory offset in
// locals[localIdx]. This can't exceed available memory due to previous
// validation in compileMustValidMemOffset.
func compileStore(localOffsetIdx, localValIdx byte, outType api.ValueType) []byte {
	body := []byte{
		wasm.OpcodeLocalSet, localValIdx, // stack: []
		wasm.OpcodeLocalGet, localOffsetIdx, // stack: [offset]
		wasm.OpcodeLocalGet, localValIdx, // stack: [offset, v]
	}
	switch outType {
	case i32:
		return append(body, wasm.OpcodeI32Store, 0x2, 0x0) // stack: []
	case i64:
		return append(body, wasm.OpcodeI64Store, 0x3, 0x0) // stack: []
	default:
		panic(fmt.Errorf("TODO: unsupported outType: %v", outType))
	}
}
