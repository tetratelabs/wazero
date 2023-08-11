package arm64

import (
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
)

// EmitGoEntryPreamble implements backend.FunctionABI. This assumes `entrypoint` function (in abi_go_entry_arm64.s) passes:
//
//  1. First (execution context ptr) and Second arguments are already passed in x0, and x1.
//  2. param/result slice ptr in x19; the pointer to []uint64{} which is used to pass arguments and accept return values.
//  3. Go-allocated stack slice ptr in x26.
//
// also SP and FP are correct Go-runtime-based values, and LR is the return address to the Go-side caller.
func (a *abiImpl) EmitGoEntryPreamble() {
	root := a.constructGoEntryPreamble()
	a.m.encode(root)
}

var (
	executionContextPtrReg = x0VReg
	// callee-saved regs so that they can be used in the prologue and epilogue.
	savedExecutionContextPtr = x18VReg
	paramResultSlicePtr      = v19VReg
	// goAllocatedStackPtr is not used in the epilogue.
	goAllocatedStackPtr = x26VReg
)

func (a *abiImpl) constructGoEntryPreamble() (root *instruction) {
	m := a.m
	root = m.allocateNop()

	//// ----------------------------------- prologue ----------------------------------- ////

	// First, we save executionContextPtrReg into a callee-saved register so that it can be used in epilogue as well.
	// 		mov x18, x0
	cur := a.move64(savedExecutionContextPtr, executionContextPtrReg, root)

	// Next, save the current FP, SP and LR into the wazevo.executionContext:
	// 		str fp, [savedExecutionContextPtr, #OriginalFramePointer]
	//      mov tmp, sp ;; sp cannot be str'ed directly.
	// 		str sp, [savedExecutionContextPtr, #OriginalStackPointer]
	// 		str lr, [savedExecutionContextPtr, #GoReturnAddress]
	cur = a.loadOrStoreAtExecutionContext(fpVReg, wazevoapi.ExecutionContextOffsets.OriginalFramePointer, true, cur)
	cur = a.move64(tmpRegVReg, spVReg, cur)
	cur = a.loadOrStoreAtExecutionContext(tmpRegVReg, wazevoapi.ExecutionContextOffsets.OriginalStackPointer, true, cur)
	cur = a.loadOrStoreAtExecutionContext(lrVReg, wazevoapi.ExecutionContextOffsets.GoReturnAddress, true, cur)

	// Next, adjust the Go-allocated stack pointer to reserve the arg/result spaces.
	// 		sub x28, x28, #stackSlotSize
	if stackSlotSize := a.alignedStackSlotSize(); stackSlotSize > 0 {
		if imm12Operand, ok := asImm12Operand(uint64(stackSlotSize)); ok {
			instr := m.allocateInstr()
			rd := operandNR(goAllocatedStackPtr)
			instr.asALU(aluOpSub, rd, rd, imm12Operand, true)
			cur = linkInstr(cur, instr)
		} else {
			panic("TODO: too large stack slot size")
		}
	}

	// Then, move the Go-allocated stack pointer to SP:
	// 		mov sp, x28
	cur = a.move64(spVReg, goAllocatedStackPtr, cur)

	for i := range a.args {
		if i < 2 {
			// module context ptr and execution context ptr are passed in x0 and x1 by the Go assembly function.
			continue
		}
		offset := int64((i - 2) * 8) // Each param is passed in as uint64, so *8.
		arg := &a.args[i]
		switch arg.Kind {
		case backend.ABIArgKindReg:
			r := arg.Reg
			rd, mode := operandNR(r), addressMode{
				kind: addressModeKindRegUnsignedImm12,
				rn:   paramResultSlicePtr,
				imm:  offset,
			}

			instr := m.allocateInstr()
			switch arg.Type {
			case ssa.TypeI32:
				instr.asULoad(rd, mode, 32)
			case ssa.TypeI64:
				instr.asULoad(rd, mode, 64)
			case ssa.TypeF32:
				instr.asFpuLoad(rd, mode, 32)
			case ssa.TypeF64:
				instr.asFpuLoad(rd, mode, 64)
			}
			cur = linkInstr(cur, instr)
		case backend.ABIArgKindStack:
			panic("TODO")
		}
	}

	// Call the real function coming after epilogue:
	// 		bl #<offset of real function from this instruction>
	// But at this point, we don't know the size of epilogue, so we emit a placeholder.
	bl := m.allocateInstr()
	cur = linkInstr(cur, bl)

	///// ----------------------------------- epilogue ----------------------------------- /////

	// Store the register results into paramResultSlicePtr.
	for i := range a.rets {
		ret := &a.rets[i]
		switch ret.Kind {
		case backend.ABIArgKindReg:
			rd := operandNR(ret.Reg)
			mode := addressMode{kind: addressModeKindPostIndex, rn: paramResultSlicePtr, imm: 8}
			instr := m.allocateInstr()
			instr.asStore(rd, mode, ret.Type.Bits())
			cur = linkInstr(cur, instr)
		case backend.ABIArgKindStack:
			offset, typ := ret.Offset, ret.Type
			if offset != 0 && !offsetFitsInAddressModeKindRegUnsignedImm12(typ.Bits(), ret.Offset) {
				// Do we really want to support?
				panic("TODO: too many parameters")
			}

			tmpOperand := operandNR(tmpRegVReg)

			// First load the value from the Go-allocated stack into temporary.
			mode := addressMode{kind: addressModeKindRegUnsignedImm12, rn: spVReg, imm: offset}
			toTmp := m.allocateInstr()
			switch ret.Type {
			case ssa.TypeI32, ssa.TypeI64:
				toTmp.asULoad(tmpOperand, mode, typ.Bits())
			case ssa.TypeF32, ssa.TypeF64:
				toTmp.asFpuLoad(tmpOperand, mode, typ.Bits())
			default:
				panic("TODO")
			}
			cur = linkInstr(cur, toTmp)

			// Then write it back to the paramResultSlicePtr.
			mode = addressMode{kind: addressModeKindPostIndex, rn: paramResultSlicePtr, imm: 8}
			storeTmp := m.allocateInstr()
			storeTmp.asStore(tmpOperand, mode, ret.Type.Bits())
			cur = linkInstr(cur, storeTmp)
		}
	}
	// Finally, restore the FP, SP and LR, and return to the Go code.
	// 		ldr fp, [savedExecutionContextPtr, #OriginalFramePointer]
	// 		ldr tmp, [savedExecutionContextPtr, #OriginalStackPointer]
	//      mov sp, tmp ;; sp cannot be str'ed directly.
	// 		ldr lr, [savedExecutionContextPtr, #GoReturnAddress]
	// 		ret ;; --> return to the Go code
	cur = a.loadOrStoreAtExecutionContext(fpVReg, wazevoapi.ExecutionContextOffsets.OriginalFramePointer, false, cur)
	cur = a.loadOrStoreAtExecutionContext(tmpRegVReg, wazevoapi.ExecutionContextOffsets.OriginalStackPointer, false, cur)
	cur = a.move64(spVReg, tmpRegVReg, cur)
	cur = a.loadOrStoreAtExecutionContext(lrVReg, wazevoapi.ExecutionContextOffsets.GoReturnAddress, false, cur)
	retInst := a.m.allocateInstr()
	retInst.asRet(nil)
	linkInstr(cur, retInst)

	///// ----------------------------------- epilogue end / real function begins ----------------------------------- /////

	// Now that we allocated all instructions needed, we can calculate the size of epilogue, and finalize the
	// bl instruction to call the real function coming after epilogue.
	var blAt, realFuncAt int64
	for _cur := root; _cur != nil; _cur = _cur.next {
		if _cur == bl {
			blAt = realFuncAt
		}
		realFuncAt += _cur.size()
	}
	bl.asCallImm(realFuncAt - blAt)
	return
}

func (a *abiImpl) move64(dst, src regalloc.VReg, prev *instruction) *instruction {
	instr := a.m.allocateInstr()
	instr.asMove64(dst, src)
	return linkInstr(prev, instr)
}

func (a *abiImpl) loadOrStoreAtExecutionContext(d regalloc.VReg, offset wazevoapi.Offset, store bool, prev *instruction) *instruction {
	instr := a.m.allocateInstr()
	mode := addressMode{kind: addressModeKindRegUnsignedImm12, rn: savedExecutionContextPtr, imm: offset.I64()}
	if store {
		instr.asStore(operandNR(d), mode, 64)
	} else {
		instr.asULoad(operandNR(d), mode, 64)
	}
	return linkInstr(prev, instr)
}

func linkInstr(prev, next *instruction) *instruction {
	prev.next = next
	next.prev = prev
	return next
}
