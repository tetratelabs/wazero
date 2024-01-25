package amd64

import (
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
)

var calleeSavedVRegs = []regalloc.VReg{
	rdxVReg, r12VReg, r13VReg, r14VReg, r15VReg,
	xmm8VReg, xmm9VReg, xmm10VReg, xmm11VReg, xmm12VReg, xmm13VReg, xmm14VReg, xmm15VReg,
}

// CompileGoFunctionTrampoline implements backend.Machine.
func (m *machine) CompileGoFunctionTrampoline(exitCode wazevoapi.ExitCode, sig *ssa.Signature, needModuleContextPtr bool) []byte {
	exct := m.ectx
	argBegin := 1 // Skips exec context by default.
	if needModuleContextPtr {
		argBegin++
	}

	abi := &backend.FunctionABI{}
	abi.Init(sig, intArgResultRegs, floatArgResultRegs)
	m.currentABI = abi

	cur := m.allocateNop()
	exct.RootInstr = cur

	// Execution context is always the first argument.
	execCtrPtr := raxVReg

	// First we update RBP and RSP just like the normal prologue.
	//
	//                   (high address)                     (high address)
	//       RBP ----> +-----------------+                +-----------------+
	//                 |     .......     |                |     .......     |
	//                 |      ret Y      |                |      ret Y      |
	//                 |     .......     |                |     .......     |
	//                 |      ret 0      |                |      ret 0      |
	//                 |      arg X      |                |      arg X      |
	//                 |     .......     |     ====>      |     .......     |
	//                 |      arg 1      |                |      arg 1      |
	//                 |      arg 0      |                |      arg 0      |
	//                 |   Return Addr   |                |   Return Addr   |
	//       RSP ----> +-----------------+                |    Caller_RBP   |
	//                    (low address)                   +-----------------+ <----- RSP, RBP
	//
	cur = m.setupRBPRSP(cur)

	goSliceSizeAligned, goSliceSizeAlignedUnaligned := backend.GoFunctionCallRequiredStackSize(sig, argBegin)
	if !m.stackBoundsCheckDisabled { //nolint
		// TODO: stack bounds check
	}

	// Save the callee saved registers.
	cur = m.saveRegistersInExecutionContext(cur, execCtrPtr, calleeSavedVRegs)

	if needModuleContextPtr {
		moduleCtrPtr := rcxVReg // Module context is always the second argument.
		mem := newAmodeImmReg(
			wazevoapi.ExecutionContextOffsetGoFunctionCallCalleeModuleContextOpaque.U32(),
			execCtrPtr)
		store := m.allocateInstr().asMovRM(moduleCtrPtr, newOperandMem(mem), 8)
		cur = linkInstr(cur, store)
	}

	// Now let's advance the RSP to the stack slot for the arguments.
	//
	//                (high address)                     (high address)
	//              +-----------------+               +-----------------+
	//              |     .......     |               |     .......     |
	//              |      ret Y      |               |      ret Y      |
	//              |     .......     |               |     .......     |
	//              |      ret 0      |               |      ret 0      |
	//              |      arg X      |               |      arg X      |
	//              |     .......     |   =======>    |     .......     |
	//              |      arg 1      |               |      arg 1      |
	//              |      arg 0      |               |      arg 0      |
	//              |   Return Addr   |               |   Return Addr   |
	//              |    Caller_RBP   |               |    Caller_RBP   |
	//  RBP,RSP --> +-----------------+               +-----------------+ <----- RBP
	//                 (low address)                  |  arg[N]/ret[M]  |
	//                                                |    ..........   |
	//                                                |  arg[1]/ret[1]  |
	//                                                |  arg[0]/ret[0]  |
	//                                                +-----------------+ <----- RSP
	//                                                   (low address)
	//
	// where the region of "arg[0]/ret[0] ... arg[N]/ret[M]" is the stack used by the Go functions,
	// therefore will be accessed as the usual []uint64. So that's where we need to pass/receive
	// the arguments/return values to/from Go function.
	cur = m.addRSP(-int32(goSliceSizeAligned), cur)

	// Next, we need to store all the arguments to the stack in the typical Wasm stack style.
	var offsetInGoSlice int32
	for i := range abi.Args[argBegin:] {
		arg := &abi.Args[argBegin+i]
		var v regalloc.VReg
		if arg.Kind == backend.ABIArgKindReg {
			v = arg.Reg
		} else {
			panic("TODO: stack arguments")
		}

		store := m.allocateInstr()
		mem := newOperandMem(newAmodeImmReg(uint32(offsetInGoSlice), rspVReg))
		switch arg.Type {
		case ssa.TypeI32:
			store.asMovRM(v, mem, 4)
			offsetInGoSlice += 8 // always uint64 rep.
		case ssa.TypeI64:
			store.asMovRM(v, mem, 8)
			offsetInGoSlice += 8
		case ssa.TypeF32:
			store.asXmmMovRM(sseOpcodeMovss, v, mem)
			offsetInGoSlice += 8 // always uint64 rep.
		case ssa.TypeF64:
			store.asXmmMovRM(sseOpcodeMovsd, v, mem)
			offsetInGoSlice += 8
		case ssa.TypeV128:
			store.asXmmMovRM(sseOpcodeMovdqu, v, mem)
			offsetInGoSlice += 16
		default:
			panic("BUG")
		}
		cur = linkInstr(cur, store)
	}

	// Finally we push the size of the slice to the stack so the stack looks like:
	//
	//          (high address)
	//       +-----------------+
	//       |     .......     |
	//       |      ret Y      |
	//       |     .......     |
	//       |      ret 0      |
	//       |      arg X      |
	//       |     .......     |
	//       |      arg 1      |
	//       |      arg 0      |
	//       |   Return Addr   |
	//       |    Caller_RBP   |
	//       +-----------------+ <----- RBP
	//       |  arg[N]/ret[M]  |
	//       |    ..........   |
	//       |  arg[1]/ret[1]  |
	//       |  arg[0]/ret[0]  |
	//       |    slice size   |
	//       +-----------------+ <----- RSP
	//         (low address)
	//
	// 		push $sliceSize
	cur = linkInstr(cur, m.allocateInstr().asPush64(newOperandImm32(uint32(goSliceSizeAlignedUnaligned))))

	// Load the exitCode to the register.
	exitCodeReg := r12VReg // Callee saved which is already saved.
	cur = linkInstr(cur, m.allocateInstr().asImm(exitCodeReg, uint64(exitCode), false))

	setExitCode, saveRsp, saveRbp := m.allocateExitInstructions(execCtrPtr, exitCodeReg)
	cur = linkInstr(cur, setExitCode)
	cur = linkInstr(cur, saveRsp)
	cur = linkInstr(cur, saveRbp)

	// Ready to exit the execution.
	cur = m.storeReturnAddressAndExit(cur, execCtrPtr)

	// After the call, we need to restore the callee saved registers.
	cur = m.restoreRegistersInExecutionContext(cur, execCtrPtr, calleeSavedVRegs)

	// We don't need the slice size anymore, so pop it.
	cur = m.addRSP(8, cur)

	// Ready to set up the results.
	offsetInGoSlice = 0
	for i := range abi.Rets {
		r := &abi.Rets[i]
		if r.Kind == backend.ABIArgKindReg {
			v := r.Reg
			load := m.allocateInstr()
			mem := newOperandMem(newAmodeImmReg(uint32(offsetInGoSlice), rspVReg))
			switch r.Type {
			case ssa.TypeI32:
				load.asMovzxRmR(extModeLQ, mem, v)
				offsetInGoSlice += 8 // always uint64 rep.
			case ssa.TypeI64:
				load.asMov64MR(mem, v)
				offsetInGoSlice += 8
			case ssa.TypeF32:
				load.asXmmUnaryRmR(sseOpcodeMovss, mem, v)
				offsetInGoSlice += 8 // always uint64 rep.
			case ssa.TypeF64:
				load.asXmmUnaryRmR(sseOpcodeMovsd, mem, v)
				offsetInGoSlice += 8
			case ssa.TypeV128:
				load.asXmmUnaryRmR(sseOpcodeMovdqu, mem, v)
				offsetInGoSlice += 16
			default:
				panic("BUG")
			}
			cur = linkInstr(cur, load)
		} else {
			panic("TODO: stack results")
		}
	}

	// Finally ready to return.
	cur = m.revertRBPRSP(cur)
	linkInstr(cur, m.allocateInstr().asRet(nil))

	m.encodeWithoutSSA(exct.RootInstr)
	return m.c.Buf()
}

func (m *machine) saveRegistersInExecutionContext(cur *instruction, execCtx regalloc.VReg, regs []regalloc.VReg) *instruction {
	offset := wazevoapi.ExecutionContextOffsetSavedRegistersBegin.I64()
	for _, v := range regs {
		store := m.allocateInstr()
		mem := newOperandMem(newAmodeImmReg(uint32(offset), execCtx))
		switch v.RegType() {
		case regalloc.RegTypeInt:
			store.asMovRM(v, mem, 8)
		case regalloc.RegTypeFloat:
			store.asXmmMovRM(sseOpcodeMovdqu, v, mem)
		default:
			panic("BUG")
		}
		cur = linkInstr(cur, store)
		offset += 16 // See execution context struct. Each register is 16 bytes-aligned unconditionally.
	}
	return cur
}

func (m *machine) restoreRegistersInExecutionContext(cur *instruction, execCtx regalloc.VReg, regs []regalloc.VReg) *instruction {
	offset := wazevoapi.ExecutionContextOffsetSavedRegistersBegin.I64()
	for _, v := range regs {
		load := m.allocateInstr()
		mem := newOperandMem(newAmodeImmReg(uint32(offset), execCtx))
		switch v.RegType() {
		case regalloc.RegTypeInt:
			load.asMov64MR(mem, v)
		case regalloc.RegTypeFloat:
			load.asXmmUnaryRmR(sseOpcodeMovdqu, mem, v)
		default:
			panic("BUG")
		}
		cur = linkInstr(cur, load)
		offset += 16 // See execution context struct. Each register is 16 bytes-aligned unconditionally.
	}
	return cur
}

func (m *machine) storeReturnAddressAndExit(cur *instruction, execCtx regalloc.VReg) *instruction {
	readRip := m.allocateInstr()
	cur = linkInstr(cur, readRip)

	ripReg := r12VReg // Callee saved which is already saved.
	saveRip := m.allocateInstr().asMovRM(
		ripReg,
		newOperandMem(newAmodeImmReg(wazevoapi.ExecutionContextOffsetGoCallReturnAddress.U32(), execCtx)),
		8,
	)
	cur = linkInstr(cur, saveRip)

	exit := m.allocateInstr().asExitSeq(execCtx)
	cur = linkInstr(cur, exit)

	nop, l := m.allocateBrTarget()
	cur = linkInstr(cur, nop)
	readRip.asLEA(newAmodeRipRelative(l), ripReg)
	return cur
}
