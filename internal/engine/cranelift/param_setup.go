package cranelift

import (
	"bytes"
	"fmt"
	"runtime"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/asm/arm64"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// paramSetupFn returns the machine code to set up registers and stack with the parameters
// passed via originalParams []uin64 (the last argument to entrypoints).
// The returned machine code is executed right before jumping into in-Wasm machine code.
// See entrypoints_arm64.s for detail.
func (e *engine) paramSetupFn(tp *wasm.FunctionType) (ret []byte, err error) {
	if runtime.GOARCH == "arm64" {
		return e.paramSetupFnArm64(tp)
	} else if runtime.GOARCH == "amd64" {
		return e.paramSetupFnAmd64(tp)
	} else {
		panic(fmt.Sprintf("BUG: unsupported GOARCH: %s", runtime.GOARCH))
	}
}

// paramSetupFnArm64 implements paramSetupFn for arm64 architecture.
func (e *engine) paramSetupFnArm64(tp *wasm.FunctionType) (ret []byte, err error) {
	const (
		// paramRegsCounts is the number of parameter registers in the arm64 calling convention.
		paramGpRegsStart     = 2 // x2.
		paramGpRegsCounts    = 6 // x2-x7. (x0 and x1 are for vm contexts.)
		paramFloatRegsCounts = 8 // v0-v7.
		// This must be matched with PARAMS_REGISTER in entrypoints_arm64.s.
		paramsRegister = arm64.RegR10
		// Anything unreserved in entrypoints_arm64.s is fine.
		temporaryRegister = arm64.RegR27
	)

	if len(tp.Params) == 0 {
		panic("BUG: this should be handle without entering paramSetupFn")
	}

	key := tp.String()
	if ret, ok := e.paramsSetupCodes[key]; ok {
		return ret, nil
	}

	paramTypes := tp.Params
	assembler := arm64.NewAssembler(temporaryRegister)

	var paramsSliceOffset int64
	intCount, floatCount := paramGpRegsStart, 0
	var needIntStackAllocation, needFloatStackAllocation bool
	// TODO: Use LDP to reduce # of instructions whenever possible.
	for _, vt := range paramTypes {
		switch vt {
		case wasm.ValueTypeI32:
			if intCount <= paramGpRegsCounts {
				assembler.CompileMemoryToRegister(
					arm64.LDRW, paramsRegister, paramsSliceOffset,
					arm64.RegR0+asm.Register(intCount))
			} else {
				// There's no registers left for parameters.
				needIntStackAllocation = true
			}
			intCount++
		case wasm.ValueTypeI64:
			if intCount <= paramGpRegsCounts {
				assembler.CompileMemoryToRegister(
					arm64.LDRD, paramsRegister, paramsSliceOffset,
					arm64.RegR0+asm.Register(intCount))
			} else {
				// There's no registers left for parameters.
				needIntStackAllocation = true
			}
			intCount++
		case wasm.ValueTypeF32:
			if floatCount != paramFloatRegsCounts {
				assembler.CompileMemoryToRegister(
					arm64.FLDRS, paramsRegister, paramsSliceOffset,
					arm64.RegV0+asm.Register(floatCount))
			} else {
				// There's no registers left for parameters.
				needFloatStackAllocation = true
			}
			floatCount++
		case wasm.ValueTypeF64:
			if floatCount != paramFloatRegsCounts {
				assembler.CompileMemoryToRegister(
					arm64.FLDRD, paramsRegister, paramsSliceOffset,
					arm64.RegV0+asm.Register(floatCount))
			} else {
				// There's no registers left for parameters.
				needFloatStackAllocation = true
			}
			floatCount++
		default:
			panic("TODO")
		}

		// In original param slices, all params are uint64.
		paramsSliceOffset += 8
	}

	multiResults := len(tp.Results) > 1
	if multiResults {
		// In case of multiple results, we have to pass the pointer to the &paramResults[0] as the last argument.
		panic("TODO")
	}

	if needIntStackAllocation || needFloatStackAllocation {
		// Note: all the integer comes first then, float comes next, and finally the pointer to the result
		// array if this is the multi results (according to the undocumented cranelift calling convention,
		// we can verify it by looking at the disassembly of machine code directly).
		// TODO: investigate how to handle stack pointer's alignment on 16 bytes boundary.
		panic("TODO")
	}

	assembler.CompileJumpToRegister(arm64.RET, arm64.RegR30)

	raw, err := assembler.Assemble()
	if err != nil {
		return nil, err
	}

	ret, err = platform.MmapCodeSegment(bytes.NewReader(raw), len(raw))
	if err != nil {
		return nil, err
	}
	e.paramsSetupCodes[key] = ret
	return
}

// paramSetupFnAmd64 implements paramSetupFn for arm64 architecture.
func (e *engine) paramSetupFnAmd64(tp *wasm.FunctionType) (ret []byte, err error) {
	panic("TODO")
}
