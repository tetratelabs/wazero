package cranelift

import (
	"github.com/tetratelabs/wazero/internal/wasm"
)

// TODO: document
// - paramSetupExecutableAddr is ignored by no param functions.
// - originalParams is ignored by no param functions.
type entryPointFn func(vmCtx *vmContext, functionAddress *byte, stack uintptr, results *byte, paramSetupExecutableAddr *byte, originalParams *uint64)

// The followings are implemented in entrypoints_arm64.s/entrypoints_amd64.s.

func entryPointNoParamNoResult(*vmContext, *byte, uintptr, *byte, *byte, *uint64)
func entryPointNoParamI32Result(*vmContext, *byte, uintptr, *byte, *byte, *uint64)
func entryPointNoParamI64Result(*vmContext, *byte, uintptr, *byte, *byte, *uint64)
func entryPointNoParamF32Result(*vmContext, *byte, uintptr, *byte, *byte, *uint64)
func entryPointNoParamF64Result(*vmContext, *byte, uintptr, *byte, *byte, *uint64)
func entryPointNoParamI32PlusMultiResult(*vmContext, *byte, uintptr, *byte, *byte, *uint64)
func entryPointNoParamI64PlusMultiResult(*vmContext, *byte, uintptr, *byte, *byte, *uint64)
func entryPointNoParamF32PlusMultiResult(*vmContext, *byte, uintptr, *byte, *byte, *uint64)
func entryPointNoParamF64PlusMultiResult(*vmContext, *byte, uintptr, *byte, *byte, *uint64)

func entryPointWithParamNoResult(*vmContext, *byte, uintptr, *byte, *byte, *uint64)
func entryPointWithParamI32Result(*vmContext, *byte, uintptr, *byte, *byte, *uint64)
func entryPointWithParamI64Result(*vmContext, *byte, uintptr, *byte, *byte, *uint64)
func entryPointWithParamF32Result(*vmContext, *byte, uintptr, *byte, *byte, *uint64)
func entryPointWithParamF64Result(*vmContext, *byte, uintptr, *byte, *byte, *uint64)
func entryPointWithParamI32PlusMultiResult(*vmContext, *byte, uintptr, *byte, *byte, *uint64)
func entryPointWithParamI64PlusMultiResult(*vmContext, *byte, uintptr, *byte, *byte, *uint64)
func entryPointWithParamF32PlusMultiResult(*vmContext, *byte, uintptr, *byte, *byte, *uint64)
func entryPointWithParamF64PlusMultiResult(*vmContext, *byte, uintptr, *byte, *byte, *uint64)

// Static checks on Go assembly function signatures.
var (
	// No params.
	_ entryPointFn = entryPointNoParamNoResult
	_ entryPointFn = entryPointNoParamI32Result
	_ entryPointFn = entryPointNoParamF32Result
	_ entryPointFn = entryPointNoParamI64Result
	_ entryPointFn = entryPointNoParamF64Result
	_ entryPointFn = entryPointNoParamI32PlusMultiResult
	_ entryPointFn = entryPointNoParamF32PlusMultiResult
	_ entryPointFn = entryPointNoParamI64PlusMultiResult
	_ entryPointFn = entryPointNoParamF64PlusMultiResult
	// With params.
	_ entryPointFn = entryPointWithParamNoResult
	_ entryPointFn = entryPointWithParamI32Result
	_ entryPointFn = entryPointWithParamI64Result
	_ entryPointFn = entryPointWithParamF32Result
	_ entryPointFn = entryPointWithParamF64Result
	_ entryPointFn = entryPointWithParamI32PlusMultiResult
	_ entryPointFn = entryPointWithParamI64PlusMultiResult
	_ entryPointFn = entryPointWithParamF32PlusMultiResult
	_ entryPointFn = entryPointWithParamF64PlusMultiResult
)

// getEntryPoint returns the entryPointFn appropriate for the given wasm.FunctionType.
// This is called per callEngine creation.
func getEntryPoint(typ *wasm.FunctionType) entryPointFn {
	if len(typ.Params) == 0 {
		switch len(typ.Results) {
		case 0:
			return entryPointNoParamNoResult
		case 1:
			switch typ.Results[0] {
			case wasm.ValueTypeI32:
				return entryPointNoParamI32Result
			case wasm.ValueTypeF32:
				return entryPointNoParamF32Result
			case wasm.ValueTypeI64:
				return entryPointNoParamI64Result
			case wasm.ValueTypeF64:
				return entryPointNoParamF64Result
			default:
				panic("TODO")
			}
		default:
			switch typ.Results[0] {
			case wasm.ValueTypeI32:
				return entryPointNoParamI32PlusMultiResult
			case wasm.ValueTypeF32:
				return entryPointNoParamF32PlusMultiResult
			case wasm.ValueTypeI64:
				return entryPointNoParamI64PlusMultiResult
			case wasm.ValueTypeF64:
				return entryPointNoParamF64PlusMultiResult
			default:
				panic("TODO")
			}
		}
	} else {
		switch len(typ.Results) {
		case 0:
			return entryPointWithParamNoResult
		case 1:
			switch typ.Results[0] {
			case wasm.ValueTypeI32:
				return entryPointWithParamI32Result
			case wasm.ValueTypeF32:
				return entryPointWithParamF32Result
			case wasm.ValueTypeI64:
				return entryPointWithParamI64Result
			case wasm.ValueTypeF64:
				return entryPointWithParamF64Result
			default:
				panic("TODO")
			}
		default:
			switch typ.Results[0] {
			case wasm.ValueTypeI32:
				return entryPointWithParamI32PlusMultiResult
			case wasm.ValueTypeF32:
				return entryPointWithParamF32PlusMultiResult
			case wasm.ValueTypeI64:
				return entryPointWithParamI64PlusMultiResult
			case wasm.ValueTypeF64:
				return entryPointWithParamF64PlusMultiResult
			default:
				panic("TODO")
			}
		}
	}
}
