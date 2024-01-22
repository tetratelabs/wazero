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
	paramResultSlicePtr = r12VReg
	// goAllocatedStackPtr is not used in the epilogue.
	goAllocatedStackPtr = rbxVReg
	// functionExecutable is not used in the epilogue.
	functionExecutable = rdiVReg
)

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

	if stackSlotSize := abi.AlignedArgResultStackSlotSize(); stackSlotSize > 0 {
		// Allocate stack slots for the arguments and return values.
		// 		sub $stackSlotSize, %rsp
		spDec := m.allocateInstr().asAluRmiR(aluRmiROpcodeSub, newOperandImm32(uint32(stackSlotSize)), rspVReg, true)
		cur = linkInstr(cur, spDec)
	}

	var offset uint32
	for i := range abi.Args {
		if i < 2 {
			// module context ptr and execution context ptr are passed in x0 and x1 by the Go assembly function.
			continue
		}
		arg := &abi.Args[i]
		cur = m.goEntryPreamblePassArg(cur, paramResultSlicePtr, offset, arg)
		if arg.Type == ssa.TypeV128 {
			offset += 16
		} else {
			offset += 8
		}
	}

	// Now ready to call the real function. Note that at this point stack pointer is already set to the Go-allocated,
	// which is aligned to 16 bytes.
	call := m.allocateInstr().asCallIndirect(newOperandReg(functionExecutable), &abi)
	cur = linkInstr(cur, call)

	//// ----------------------------------- epilogue ----------------------------------- ////

	// Read the results from regs and the stack, and set them correctly into the paramResultSlicePtr.
	offset = 0
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

func (m *machine) goEntryPreamblePassArg(cur *instruction, paramSlicePtr regalloc.VReg, offsetInParamSlice uint32, arg *backend.ABIArg) *instruction {
	dst := arg.Reg
	if arg.Kind == backend.ABIArgKindStack {
		// Temporary reg.
		panic("TODO")
	}

	load := m.allocateInstr()
	a := newOperandMem(newAmodeImmReg(offsetInParamSlice, paramSlicePtr))
	switch arg.Type {
	case ssa.TypeI32:
		load.asMovzxRmR(extModeLQ, a, dst)
	case ssa.TypeI64:
		load.asMov64MR(a, dst)
	case ssa.TypeF32:
		load.asXmmUnaryRmR(sseOpcodeMovss, a, dst)
	case ssa.TypeF64:
		load.asXmmUnaryRmR(sseOpcodeMovsd, a, dst)
	case ssa.TypeV128:
		load.asXmmUnaryRmR(sseOpcodeMovdqu, a, dst)
	}

	if arg.Kind == backend.ABIArgKindStack {
		// store back.
		panic("TODO")
	}

	return linkInstr(cur, load)
}

func (m *machine) goEntryPreamblePassResult(cur *instruction, resultSlicePtr regalloc.VReg, offset uint32, result *backend.ABIArg) *instruction {
	if result.Kind == backend.ABIArgKindStack {
		panic("TODO")
	}

	r := result.Reg

	store := m.allocateInstr()
	a := newOperandMem(newAmodeImmReg(offset, resultSlicePtr))
	switch result.Type {
	case ssa.TypeI32:
		store.asMovRM(r, a, 4)
	case ssa.TypeI64:
		store.asMovRM(r, a, 8)
	case ssa.TypeF32:
		store.asXmmMovRM(sseOpcodeMovss, r, a)
	case ssa.TypeF64:
		store.asXmmMovRM(sseOpcodeMovsd, r, a)
	case ssa.TypeV128:
		store.asXmmMovRM(sseOpcodeMovdqu, r, a)
	}

	return linkInstr(cur, store)
}
