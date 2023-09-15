package arm64

import (
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
)

var calleeSavedRegistersPlusLinkRegSorted = []regalloc.VReg{
	x19VReg, x20VReg, x21VReg, x22VReg, x23VReg, x24VReg, x25VReg, x26VReg, x28VReg, lrVReg,
	v18VReg, v19VReg, v20VReg, v21VReg, v22VReg, v23VReg, v24VReg, v25VReg, v26VReg, v27VReg, v28VReg, v29VReg, v30VReg, v31VReg,
}

// CompileGoFunctionTrampoline implements backend.Machine.
func (m *machine) CompileGoFunctionTrampoline(exitCode wazevoapi.ExitCode, sig *ssa.Signature, needModuleContextPtr bool) []byte {
	argBegin := 1 // Skips exec context by default.
	if needModuleContextPtr {
		argBegin++
	}

	abi := &abiImpl{m: m}
	abi.init(sig)
	m.currentABI = abi

	cur := m.allocateInstr()
	cur.asNop0()
	m.rootInstr = cur

	// Execution context is always the first argument.
	execCtrPtr := x0VReg

	// In the following, we create the following stack frame:
	//
	//                   (high address)
	//                +-----------------+
	//                |     .......     |
	//                |      ret Y      |  <----+
	//                |     .......     |       |
	//                |      ret 0      |       |
	//                |      arg X      |       |  size_of_arg_ret
	//                |     .......     |       |
	//                |      arg 1      |       |
	//     SP ------> |      arg 0      |  <----+
	//                | size_of_arg_ret |
	//                |  ReturnAddress  |
	//                +-----------------+ <----+
	//                |  arg[N]/ret[M]  |      |
	//                |   ............  |      | goCallStackSize
	//                |  arg[1]/ret[1]  |      |
	//                |  arg[0]/ret[0]  | <----+
	//                |     xxxxxx      |  ;; unused space to make it 16-byte aligned.
	//                |   frame_size    |
	//                +-----------------+
	//                   (low address)
	//
	// where the region of "arg[0]/ret[0] ... arg[N]/ret[M]" is the stack used by the Go functions,
	// therefore will be accessed as the usual []uint64. So that's where we need to pass/receive
	// the arguments/return values.

	const callFrameSize = 32 // == frame_size + xxxxxx + ReturnAddress + size_of_arg_ret.

	// First of all, we should allocate the stack for the Go function call if necessary.
	goCallStackSize := goFunctionCallRequiredStackSize(sig, argBegin)
	cur = m.insertStackBoundsCheck(goCallStackSize+callFrameSize, cur)

	// Next is to create "ReturnAddress + size_of_arg_ret".
	cur = m.createReturnAddrAndSizeOfArgRetSlot(cur)

	// Save the callee saved registers.
	cur = m.saveRegistersInExecutionContext(cur, calleeSavedRegistersPlusLinkRegSorted)

	// Next, we need to store all the arguments to the stack in the typical Wasm stack style.
	if needModuleContextPtr {
		offset := wazevoapi.ExecutionContextOffsetGoFunctionCallCalleeModuleContextOpaque.I64()
		if !offsetFitsInAddressModeKindRegUnsignedImm12(64, offset) {
			panic("BUG: too large or un-aligned offset for goFunctionCallCalleeModuleContextOpaque in execution context")
		}

		// Module context is always the second argument.
		moduleCtrPtr := x1VReg
		store := m.allocateInstr()
		amode := addressMode{kind: addressModeKindRegUnsignedImm12, rn: execCtrPtr, imm: offset}
		store.asStore(operandNR(moduleCtrPtr), amode, 64)
		cur = linkInstr(cur, store)
	}

	// Advances the stack pointer.
	cur = m.addsAddOrSubStackPointer(cur, spVReg, goCallStackSize, false, true)

	// Copy the pointer to x15VReg.
	stackPtrReg := x15VReg // Caller save, so we can use it for whatever we want.
	copySp := m.allocateInstr()
	copySp.asMove64(stackPtrReg, spVReg)
	cur = linkInstr(cur, copySp)

	for _, arg := range abi.args[argBegin:] {
		if arg.Kind == backend.ABIArgKindReg {
			store := m.allocateInstr()
			v := arg.Reg
			var sizeInBits byte
			switch arg.Type {
			case ssa.TypeI32, ssa.TypeF32, ssa.TypeI64, ssa.TypeF64:
				sizeInBits = 64 // We use uint64 for all basic types, except SIMD v128.
			default:
				panic("TODO? do you really need to pass v128 to host function?")
			}
			store.asStore(operandNR(v),
				addressMode{
					kind: addressModeKindPostIndex,
					rn:   stackPtrReg, imm: int64(sizeInBits / 8),
				}, sizeInBits)
			cur = linkInstr(cur, store)
		} else {
			// We can temporarily load the argument to the tmp register, and then store it to the stack.
			panic("TODO: many params for Go function call")
		}
	}

	// Finally, now that we've advanced SP to arg[0]/ret[0], we allocate `frame_size + xxxxxx `.
	cur = m.createFrameSizeSlot(cur, goCallStackSize)

	// Set the exit status on the execution context.
	cur = m.setExitCode(cur, x0VReg, exitCode)

	// Save the current stack pointer.
	cur = m.saveCurrentStackPointer(cur, x0VReg)

	// Exit the execution.
	cur = m.storeReturnAddressAndExit(cur)

	// After the call, we need to restore the callee saved registers.
	cur = m.restoreRegistersInExecutionContext(cur, calleeSavedRegistersPlusLinkRegSorted)

	// Get the pointer to the arg[0]/ret[0]: We need to skip `frame_size + xxxxxx `.
	if len(abi.rets) > 0 {
		cur = m.addsAddOrSubStackPointer(cur, stackPtrReg, 16, true, true)
	}

	for i := range abi.rets {
		r := &abi.rets[i]
		if r.Kind == backend.ABIArgKindReg {
			loadIntoTmp := m.allocateInstr()
			movTmpToReg := m.allocateInstr()

			mode := addressMode{kind: addressModeKindPostIndex, rn: stackPtrReg}
			tmpRegVRegVec := v17VReg // Caller save, so we can use it for whatever we want.
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
				loadIntoTmp.asFpuLoad(operandNR(tmpRegVRegVec), mode, 32)
				movTmpToReg.asFpuMov64(r.Reg, tmpRegVRegVec)
			case ssa.TypeF64:
				mode.imm = 8 // We use uint64 for all basic types, except SIMD v128.
				loadIntoTmp.asFpuLoad(operandNR(tmpRegVRegVec), mode, 64)
				movTmpToReg.asFpuMov64(r.Reg, tmpRegVRegVec)
			default:
				panic("Do you really need to return v128 from host function?")
			}
			cur = linkInstr(cur, loadIntoTmp)
			cur = linkInstr(cur, movTmpToReg)
		} else {
			// We can temporarily load the argument to the tmp register, and then store it to the stack.
			panic("TODO: many params for Go function call")
		}
	}

	// Removes the stack frame!
	cur = m.addsAddOrSubStackPointer(cur, spVReg, goCallStackSize+callFrameSize, true, true)

	ret := m.allocateInstrAfterLowering()
	ret.asRet(nil)
	linkInstr(cur, ret)

	m.encode(m.rootInstr)
	return m.compiler.Buf()
}

func (m *machine) saveRegistersInExecutionContext(cur *instruction, regs []regalloc.VReg) *instruction {
	offset := wazevoapi.ExecutionContextOffsetSavedRegistersBegin.I64()
	for _, v := range regs {
		store := m.allocateInstrAfterLowering()
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
				// Execution context is always the first argument.
				rn: x0VReg, imm: offset,
			}, sizeInBits)
		store.prev = cur
		cur.next = store
		cur = store
		offset += 16 // Imm12 must be aligned 16 for vector regs, so we unconditionally store regs at the offset of multiple of 16.
	}
	return cur
}

func (m *machine) restoreRegistersInExecutionContext(cur *instruction, regs []regalloc.VReg) *instruction {
	offset := wazevoapi.ExecutionContextOffsetSavedRegistersBegin.I64()
	for _, v := range regs {
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
		cur = linkInstr(cur, load)
		offset += 16 // Imm12 must be aligned 16 for vector regs, so we unconditionally load regs at the offset of multiple of 16.
	}
	return cur
}

func (m *machine) lowerConstantI64AndInsert(cur *instruction, dst regalloc.VReg, v int64) *instruction {
	m.pendingInstructions = m.pendingInstructions[:0]
	m.lowerConstantI64(dst, v)
	for _, instr := range m.pendingInstructions {
		cur = linkInstr(cur, instr)
	}
	return cur
}

func (m *machine) lowerConstantI32AndInsert(cur *instruction, dst regalloc.VReg, v int32) *instruction {
	m.pendingInstructions = m.pendingInstructions[:0]
	m.lowerConstantI32(dst, v)
	for _, instr := range m.pendingInstructions {
		cur = linkInstr(cur, instr)
	}
	return cur
}

func (m *machine) setExitCode(cur *instruction, execCtr regalloc.VReg, exitCode wazevoapi.ExitCode) *instruction {
	constReg := x17VReg // caller-saved, so we can use it.
	cur = m.lowerConstantI32AndInsert(cur, constReg, int32(exitCode))

	// Set the exit status on the execution context.
	setExistStatus := m.allocateInstrAfterLowering()
	setExistStatus.asStore(operandNR(constReg),
		addressMode{
			kind: addressModeKindRegUnsignedImm12,
			rn:   execCtr, imm: wazevoapi.ExecutionContextOffsetExitCodeOffset.I64(),
		}, 32)
	cur = linkInstr(cur, setExistStatus)
	return cur
}

func (m *machine) storeReturnAddressAndExit(cur *instruction) *instruction {
	// Read the return address into tmp, and store it in the execution context.
	adr := m.allocateInstrAfterLowering()
	adr.asAdr(tmpRegVReg, exitSequenceSize+8)
	cur = linkInstr(cur, adr)

	storeReturnAddr := m.allocateInstrAfterLowering()
	storeReturnAddr.asStore(operandNR(tmpRegVReg),
		addressMode{
			kind: addressModeKindRegUnsignedImm12,
			// Execution context is always the first argument.
			rn: x0VReg, imm: wazevoapi.ExecutionContextOffsetGoCallReturnAddress.I64(),
		}, 64)
	cur = linkInstr(cur, storeReturnAddr)

	// Exit the execution.
	trapSeq := m.allocateInstrAfterLowering()
	trapSeq.asExitSequence(x0VReg)
	cur = linkInstr(cur, trapSeq)
	return cur
}

func (m *machine) saveCurrentStackPointer(cur *instruction, execCtr regalloc.VReg) *instruction {
	// Save the current stack pointer:
	// 	mov tmp, sp,
	// 	str tmp, [exec_ctx, #stackPointerBeforeGoCall]
	movSp := m.allocateInstrAfterLowering()
	movSp.asMove64(tmpRegVReg, spVReg)
	cur = linkInstr(cur, movSp)

	strSp := m.allocateInstrAfterLowering()
	strSp.asStore(operandNR(tmpRegVReg),
		addressMode{
			kind: addressModeKindRegUnsignedImm12,
			rn:   execCtr, imm: wazevoapi.ExecutionContextOffsetStackPointerBeforeGoCall.I64(),
		}, 64)
	cur = linkInstr(cur, strSp)
	return cur
}

// goFunctionCallRequiredStackSize returns the size of the stack required for the Go function call.
func goFunctionCallRequiredStackSize(sig *ssa.Signature, argBegin int) (ret int64) {
	var paramNeededInBytes, resultNeededInBytes int64
	for _, p := range sig.Params[argBegin:] {
		s := int64(p.Size())
		if s < 8 {
			s = 8 // We use uint64 for all basic types, except SIMD v128.
		}
		paramNeededInBytes += s
	}
	for _, r := range sig.Results {
		s := int64(r.Size())
		if s < 8 {
			s = 8 // We use uint64 for all basic types, except SIMD v128.
		}
		resultNeededInBytes += s
	}

	if paramNeededInBytes > resultNeededInBytes {
		ret = paramNeededInBytes
	} else {
		ret = resultNeededInBytes
	}
	// Align to 16 bytes.
	ret = (ret + 15) &^ 15
	return
}
