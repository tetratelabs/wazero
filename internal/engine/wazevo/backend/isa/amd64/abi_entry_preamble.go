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
	paramResultSliceCopied = r15VReg //nolint
	// functionExecutable is not used in the epilogue.
	functionExecutable = r14VReg
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
	cur := m.move64(savedExecutionContextPtr, executionContextPtrReg, root)

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
	if len(abi.Args) > 2 {
		panic("TODO")
	}

	// Now ready to call the real function.
	call := m.allocateInstr().asCallIndirect(newOperandReg(functionExecutable), &abi)
	cur = linkInstr(cur, call)

	//// ----------------------------------- epilogue ----------------------------------- ////

	// TODO: read the results from regs and the stack, and set them correctly into the paramResultSlicePtr.
	if len(abi.Rets) > 0 {
		panic("TODO")
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
