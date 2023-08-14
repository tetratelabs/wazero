package arm64

import (
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
)

var calleeSavedRegistersPlusLinkRegSorted = []regalloc.VReg{
	x18VReg, x19VReg, x20VReg, x21VReg, x22VReg, x23VReg, x24VReg, x25VReg, x26VReg, x28VReg, lrVReg,
	v18VReg, v19VReg, v20VReg, v21VReg, v22VReg, v23VReg, v24VReg, v25VReg, v26VReg, v27VReg, v28VReg, v29VReg, v30VReg, v31VReg,
}

// CompileGoFunctionTrampoline implements backend.Machine.
func (m *machine) CompileGoFunctionTrampoline(exitCode wazevoapi.ExitCode, sig *ssa.Signature) {
	cur := m.allocateInstr()
	cur.asNop0()
	m.rootInstr = cur

	// Execution context is always the first argument.
	execCtrPtr := x0VReg

	// Save the callee saved registers.
	offset := wazevoapi.ExecutionContextOffsets.SavedRegistersBegin.I64()
	for _, v := range calleeSavedRegistersPlusLinkRegSorted {
		store := m.allocateInstr()
		var sizeInBits byte
		switch v.RegType() {
		case regalloc.RegTypeInt:
			sizeInBits = 64
		case regalloc.RegTypeFloat:
			sizeInBits = 128
		}
		store.asStore(operandNR(v),
			addressMode{
				kind: addressModeKindRegUnsignedImm12,
				rn:   execCtrPtr, imm: offset,
			}, sizeInBits)
		store.prev = cur
		cur.next = store
		cur = store
		offset += 16 // Imm12 must be aligned 16 for vector regs, so we unconditionally store regs at the offset of multiple of 16.
	}

	// Next, we need to store all the arguments to the execution context.
	abi := m.getOrCreateABIImpl(sig)

	stackPtrReg := x15VReg // Caller save, so we can use it for whatever we want.
	imm12Op, ok := asImm12Operand(wazevoapi.ExecutionContextOffsets.GoFunctionCallStackBegin.U64())
	if !ok {
		panic("BUG: too large or un-aligned offset for Go function call stack in execution context")
	}

	goCallStackPtrCalc := m.allocateInstr()
	goCallStackPtrCalc.asALU(aluOpAdd, operandNR(stackPtrReg), operandNR(execCtrPtr), imm12Op, true)
	goCallStackPtrCalc.prev = cur
	cur.next = goCallStackPtrCalc
	cur = goCallStackPtrCalc

	for _, arg := range abi.args[1:] { // Skips exec context.
		if arg.Kind == backend.ABIArgKindReg {
			store := m.allocateInstr()
			v := arg.Reg
			var sizeInBits byte
			switch arg.Type {
			case ssa.TypeI32, ssa.TypeF32, ssa.TypeI64, ssa.TypeF64:
				sizeInBits = 64 // We use uint64 for all basic types, except SIMD v128.
			default:
				panic("TODO")
			}
			store.asStore(operandNR(v),
				addressMode{
					kind: addressModeKindPostIndex,
					rn:   stackPtrReg, imm: int64(sizeInBits / 8),
				}, sizeInBits)
			store.prev = cur
			cur.next = store
			cur = store
		} else {
			// We can temporarily load the argument to the tmp register, and then store it to the stack.
			panic("TODO: many params for Go function call")
		}
	}

	// Set the exit status on the execution context.
	cur = m.setExitCode(cur, x0VReg, exitCode)

	// Save the current stack pointer.
	cur = m.saveCurrentStackPointer(cur, x0VReg)

	// Exit the execution.
	cur = m.storeReturnAddressAndExit(cur)

	// After the call, we need to restore the callee saved registers.
	offset = wazevoapi.ExecutionContextOffsets.SavedRegistersBegin.I64()
	for _, v := range calleeSavedRegistersPlusLinkRegSorted {
		load := m.allocateInstrAfterLowering()
		var as func(dst operand, amode addressMode, sizeInBits byte)
		var sizeInBits byte
		switch v.RegType() {
		case regalloc.RegTypeInt:
			as = load.asULoad
			sizeInBits = 64
		case regalloc.RegTypeFloat:
			as = load.asFpuLoad
			sizeInBits = 128
		}
		as(operandNR(v),
			addressMode{
				kind: addressModeKindRegUnsignedImm12,
				// Execution context is always the first argument.
				rn: x0VReg, imm: offset,
			}, sizeInBits)
		load.prev = cur
		cur.next = load
		cur = load
		offset += 16 // Imm12 must be aligned 16 for vector regs, so we unconditionally load regs at the offset of multiple of 16.
	}

	// Finally, we move the results into the right places from the execution context's Go function call stack.
	goCallStackPtrCalc2 := m.allocateInstr()
	goCallStackPtrCalc2.asALU(aluOpAdd, operandNR(stackPtrReg), operandNR(execCtrPtr), imm12Op, true)
	goCallStackPtrCalc2.prev = cur
	cur.next = goCallStackPtrCalc2
	cur = goCallStackPtrCalc2

	for i := range abi.rets {
		r := &abi.rets[i]
		if r.Kind == backend.ABIArgKindReg {
			loadIntoTmp := m.allocateInstr()
			movTmpToReg := m.allocateInstr()

			mode := addressMode{kind: addressModeKindPostIndex, rn: stackPtrReg}
			switch r.Type {
			case ssa.TypeI32:
				mode.imm = 8 // We use uint64 for all basic types, except SIMD v128.
				loadIntoTmp.asULoad(operandNR(tmpRegVReg), mode, 32)
				movTmpToReg.asMove32(r.Reg, tmpRegVReg)
			case ssa.TypeI64:
				mode.imm = 8 // We use uint64 for all basic types, except SIMD v128.
				loadIntoTmp.asULoad(operandNR(tmpRegVReg), mode, 64)
				movTmpToReg.asMove64(r.Reg, tmpRegVReg)
			case ssa.TypeF32:
				mode.imm = 8 // We use uint64 for all basic types, except SIMD v128.
				loadIntoTmp.asFpuLoad(operandNR(tmpRegVReg), mode, 32)
				movTmpToReg.asFpuMov64(r.Reg, tmpRegVReg)
			case ssa.TypeF64:
				mode.imm = 8 // We use uint64 for all basic types, except SIMD v128.
				loadIntoTmp.asFpuLoad(operandNR(tmpRegVReg), mode, 64)
				movTmpToReg.asFpuMov64(r.Reg, tmpRegVReg)
			}
			loadIntoTmp.prev = cur
			cur.next = loadIntoTmp
			cur = loadIntoTmp

			movTmpToReg.prev = cur
			cur.next = movTmpToReg
			cur = movTmpToReg
		} else {
			// We can temporarily load the argument to the tmp register, and then store it to the stack.
			panic("TODO: many params for Go function call")
		}
	}

	ret := m.allocateInstrAfterLowering()
	ret.asRet(nil)
	ret.prev = cur
	cur.next = ret
}
