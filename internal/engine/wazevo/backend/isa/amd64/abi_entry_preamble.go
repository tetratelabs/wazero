package amd64

import (
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
)

var (
	executionContextPtrReg = raxVReg
	// savedExecutionContextPtr also must be a callee-saved reg so that they can be used in the prologue and epilogue.
	savedExecutionContextPtr = rdxVReg
	// paramResultSlicePtr must be callee-saved reg so that they can be used in the prologue and epilogue.
	paramResultSlicePtr = r12VReg //nolint
	// goAllocatedStackPtr is not used in the epilogue.
	goAllocatedStackPtr = rbxVReg
	// paramResultSliceCopied is not used in the epilogue.
	paramResultSliceCopied = rsiVReg //nolint
	// functionExecutable is not used in the epilogue.
	functionExecutable = rdiVReg
)

func (m *machine) goEntryPreamblePassArg(cur *instruction, paramSlicePtr regalloc.VReg, arg *backend.ABIArg, argStartOffsetFromSP int64) *instruction {
	typ := arg.Type
	bits := typ.Bits()
	isStackArg := arg.Kind == backend.ABIArgKindStack

	var loadTargetReg regalloc.VReg
	if !isStackArg {
		loadTargetReg = arg.Reg
	} else {
		switch typ {
		case ssa.TypeI32, ssa.TypeI64:
			loadTargetReg = r15VReg
		case ssa.TypeF32, ssa.TypeF64, ssa.TypeV128:
			loadTargetReg = xmm15VReg
		default:
			panic("TODO?")
		}
	}

	var postIndexImm uint32
	if typ == ssa.TypeV128 {
		postIndexImm = 16 // v128 is represented as 2x64-bit in Go slice.
	} else {
		postIndexImm = 8
	}
	loadMode := newAmodeImmReg(postIndexImm, paramSlicePtr)

	instr := m.allocateInstr()
	switch typ {
	case ssa.TypeI32:
		instr.asLEA(loadMode, loadTargetReg)
	case ssa.TypeI64:
		panic("TODO") // instr.asULoad(loadTargetReg, loadMode, 64)
	case ssa.TypeF32:
		panic("TODO") // instr.asFpuLoad(loadTargetReg, loadMode, 32)
	case ssa.TypeF64:
		panic("TODO") // instr.asFpuLoad(loadTargetReg, loadMode, 64)
	case ssa.TypeV128:
		panic("TODO") // instr.asFpuLoad(loadTargetReg, loadMode, 128)
	}
	cur = linkInstr(cur, instr)

	if isStackArg {
		panic("TODO")
		_ = bits
		//	var storeMode addressMode
		//	cur, storeMode = m.resolveAddressModeForOffsetAndInsert(cur, argStartOffsetFromSP+arg.Offset, bits, spVReg, true)
		//	toStack := m.allocateInstr()
		//	toStack.asStore(loadTargetReg, storeMode, bits)
		//	cur = linkInstr(cur, toStack)
	}
	return cur
}

// CompileEntryPreamble implements backend.Machine.
func (m *machine) CompileEntryPreamble(sig *ssa.Signature) []byte {
	root := m.compileEntryPreamble(sig)
	m.encodeWithoutRelResolution(root)
	buf := m.c.Buf()
	return buf
}

func (m *machine) compileEntryPreamble(sig *ssa.Signature) *instruction {
	abi := backend.FunctionABI{}
	abi.Init(sig, intArgResultRegs, floatArgResultRegs)

	root := m.allocateNop()

	//// ----------------------------------- prologue ----------------------------------- ////

	// First, we save executionContextPtrReg into a callee-saved register so that it can be used in epilogue as well.
	// 		mov %executionContextPtrReg, %savedExecutionContextPtr
	cur := m.move64(executionContextPtrReg, savedExecutionContextPtr, root)

	// Next is to save the original RBP and RSP into the execution context.
	// 		mov %rbp, wazevoapi.ExecutionContextOffsetOriginalFramePointer(%executionContextPtrReg)
	// 		mov %rsp, wazevoapi.ExecutionContextOffsetOriginalStackPointer(%executionContextPtrReg)
	cur = m.loadOrStore64AtExecutionCtx(executionContextPtrReg, wazevoapi.ExecutionContextOffsetOriginalFramePointer, rbpVReg, true, cur)
	cur = m.loadOrStore64AtExecutionCtx(executionContextPtrReg, wazevoapi.ExecutionContextOffsetOriginalStackPointer, rspVReg, true, cur)

	// Now set the RSP to the Go-allocated stack pointer.
	// 		mov %goAllocatedStackPtr, %rsp
	cur = m.move64(goAllocatedStackPtr, rspVReg, cur)

	// TODO: read the arguments from paramResultSliceCopied and set them correctly into the regs and stacks.
	// 	The first two args are module context ptr and execution context ptr, and they are passed in %rax and %rcx.
	prReg := paramResultSlicePtr
	if len(abi.Args) > 2 && len(abi.Rets) > 0 {
		// paramResultSlicePtr is modified during the execution of goEntryPreamblePassArg,
		// so copy it to another reg.
		cur = m.move64(paramResultSliceCopied, paramResultSlicePtr, cur)
		prReg = paramResultSliceCopied
	}

	stackSlotSize := abi.AlignedArgResultStackSlotSize()
	for i := range abi.Args {
		if i < 2 {
			// module context ptr and execution context ptr are passed in x0 and x1 by the Go assembly function.
			continue
		}
		arg := &abi.Args[i]
		cur = m.goEntryPreamblePassArg(cur, prReg, arg, -stackSlotSize)
	}

	// Now ready to call the real function. Note that at this point stack pointer is already set to the Go-allocated,
	// which is aligned to 16 bytes.
	call := m.allocateInstr().asCallIndirect(newOperandReg(functionExecutable), &abi)
	cur = linkInstr(cur, call)

	//// ----------------------------------- epilogue ----------------------------------- ////

	// Read the results from regs and the stack, and set them correctly into the paramResultSlicePtr.
	var offset uint32
	for i := range abi.Rets {
		r := &abi.Rets[i]
		cur = m.goEntryPreamblePassResult(cur, paramResultSlicePtr, offset, r)
		if r.Type == ssa.TypeV128 {
			offset += 16
		} else {
			offset += 8
		}
	}

	// Finally, restore the original RBP and RSP.
	// 		mov wazevoapi.ExecutionContextOffsetOriginalFramePointer(%executionContextPtrReg), %rbp
	// 		mov wazevoapi.ExecutionContextOffsetOriginalStackPointer(%executionContextPtrReg), %rsp
	cur = m.loadOrStore64AtExecutionCtx(savedExecutionContextPtr, wazevoapi.ExecutionContextOffsetOriginalFramePointer, rbpVReg, false, cur)
	cur = m.loadOrStore64AtExecutionCtx(savedExecutionContextPtr, wazevoapi.ExecutionContextOffsetOriginalStackPointer, rspVReg, false, cur)

	ret := m.allocateInstr().asRet(&abi)
	linkInstr(cur, ret)
	return root
}

func (m *machine) move64(src, dst regalloc.VReg, prev *instruction) *instruction {
	mov := m.allocateInstr().asMovRR(src, dst, true)
	return linkInstr(prev, mov)
}

func (m *machine) loadOrStore64AtExecutionCtx(execCtx regalloc.VReg, offset wazevoapi.Offset, r regalloc.VReg, store bool, prev *instruction) *instruction {
	mem := newOperandMem(newAmodeImmReg(offset.U32(), execCtx))
	instr := m.allocateInstr()
	if store {
		instr.asMovRM(r, mem, 8)
	} else {
		instr.asMov64MR(mem, r)
	}
	return linkInstr(prev, instr)
}

// This is for debugging.
func (m *machine) linkUD2(cur *instruction) *instruction { //nolint
	return linkInstr(cur, m.allocateInstr().asUD2())
}

func (m *machine) goEntryPreamblePassResult(cur *instruction, resultSlicePtr regalloc.VReg, offset uint32, result *backend.ABIArg) *instruction {
	if result.Kind == backend.ABIArgKindStack {
		panic("TODO")
	}

	r := result.Reg

	store := m.allocateInstr()
	switch result.Type {
	case ssa.TypeI32:
		store.asMovRM(r, newOperandMem(newAmodeImmReg(offset, resultSlicePtr)), 4)
	case ssa.TypeI64:
		store.asMovRM(r, newOperandMem(newAmodeImmReg(offset, resultSlicePtr)), 8)
	case ssa.TypeF32:
		store.asXmmMovRM(sseOpcodeMovss, r, newOperandMem(newAmodeImmReg(offset, resultSlicePtr)))
	case ssa.TypeF64:
		store.asXmmMovRM(sseOpcodeMovsd, r, newOperandMem(newAmodeImmReg(offset, resultSlicePtr)))
	case ssa.TypeV128:
		panic("TODO")
	}

	return linkInstr(cur, store)
}
