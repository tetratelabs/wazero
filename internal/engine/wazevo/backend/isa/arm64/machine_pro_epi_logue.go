package arm64

import (
	"fmt"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
)

// SetupPrologue implements backend.Machine.
func (m *machine) SetupPrologue() {
	cur := m.rootInstr
	prevInitInst := cur.next

	if !m.stackBoundsCheckDisabled {
		cur = m.insertStackBoundsCheck(m.requiredStackSize(), cur)
	}

	//
	//                   (high address)                    (high address)
	//                 +-----------------+               +------------------+
	//                 |     .......     |               |     .......      |
	//                 |      ret Y      |               |      ret Y       |
	//                 |     .......     |               |     .......      |
	//                 |      ret 0      |               |      ret 0       |
	//                 |      arg X      |               |      arg X       |
	//                 |     .......     |     ====>     |     .......      |
	//                 |      arg 1      |               |      arg 1       |
	//                 |      arg 0      |               |      arg 0       |
	//         SP----> |-----------------|               |      xxxxx       |
	//                                                   |   ret address    |
	//                                                   +------------------+ <---- SP
	//                    (low address)                     (low address)

	// Saves the return address (lr) and below the SP.
	str := m.allocateInstrAfterLowering()
	amode := addressModePreOrPostIndex(spVReg, -16 /* stack pointer must be 16-byte aligned. */, true /* decrement before store */)
	str.asStore(operandNR(lrVReg), amode, 64)
	cur.next = str
	str.prev = cur
	cur = str

	// Decrement SP if spillSlotSize > 0.
	if size := m.spillSlotSize; size > 0 {
		// Check if size is 16-byte aligned.
		if size&0xf != 0 {
			panic(fmt.Errorf("BUG: spill slot size %d is not 16-byte aligned", size))
		}

		decSP := m.allocateAddOrSubStackPointer(spVReg, size, false, true)
		cur.next = decSP
		decSP.prev = cur
		cur = decSP

		// At this point, the stack looks like:
		//
		//            (high address)
		//          +------------------+
		//          |     .......      |
		//          |      ret Y       |
		//          |     .......      |
		//          |      ret 0       |
		//          |      arg X       |
		//          |     .......      |
		//          |      arg 1       |
		//          |      arg 0       |
		//          |      xxxxx       |
		//          |   ReturnAddress  |
		//          +------------------+
		//          |   spill slot M   |
		//          |   ............   |
		//          |   spill slot 2   |
		//          |   spill slot 1   |
		//  SP----> +------------------+
		//             (low address)
	}

	if regs := m.clobberedRegs; len(regs) > 0 {
		//
		//            (high address)                  (high address)
		//          +-----------------+             +-----------------+
		//          |     .......     |             |     .......     |
		//          |      ret Y      |             |      ret Y      |
		//          |     .......     |             |     .......     |
		//          |      ret 0      |             |      ret 0      |
		//          |      arg X      |             |      arg X      |
		//          |     .......     |             |     .......     |
		//          |      arg 1      |             |      arg 1      |
		//          |      arg 0      |             |      arg 0      |
		//          |      xxxxx      |             |      xxxxx      |
		//          |   ReturnAddress |             |  ReturnAddress  |
		//          +-----------------+    ====>    +-----------------+
		//          |   ...........   |             |   ...........   |
		//          |   spill slot M  |             |   spill slot M  |
		//          |   ............  |             |   ............  |
		//          |   spill slot 2  |             |   spill slot 2  |
		//          |   spill slot 1  |             |   spill slot 1  |
		//  SP----> +-----------------+             |   spill slot 1  |
		//             (low address)                |   clobbered N   |
		//                                          |   ............  |
		//                                          |   clobbered 0   |
		//                                          +-----------------+
		//                                              (high address)
		//
		_amode := addressModePreOrPostIndex(spVReg,
			-16,  // stack pointer must be 16-byte aligned.
			true, // Decrement before store.
		)
		for _, vr := range regs {
			// TODO: pair stores to reduce the number of instructions.
			store := m.allocateInstrAfterLowering()
			store.asStore(operandNR(vr), _amode, regTypeToRegisterSizeInBits(vr.RegType()))
			cur.next = store
			store.prev = cur
			cur = store
		}
	}

	cur.next = prevInitInst

	prevInitInst.prev = cur
}

// SetupEpilogue implements backend.Machine.
func (m *machine) SetupEpilogue() {
	for cur := m.rootInstr; cur != nil; cur = cur.next {
		if cur.kind == ret {
			m.setupEpilogueAfter(cur.prev)
			continue
		}

		// Removes the redundant copy instruction.
		// TODO: doing this in `SetupEpilogue` seems weird. Find a better home.
		if cur.isCopy() && cur.rn.realReg() == cur.rd.realReg() {
			prev, next := cur.prev, cur.next
			// Remove the copy instruction.
			prev.next = next
			if next != nil {
				next.prev = prev
			}
		}
	}
}

func (m *machine) setupEpilogueAfter(cur *instruction) {
	prevNext := cur.next

	// First we need to restore the clobbered registers.
	if len(m.clobberedRegs) > 0 {
		//            (high address)
		//          +-----------------+                      +-----------------+
		//          |     .......     |                      |     .......     |
		//          |      ret Y      |                      |      ret Y      |
		//          |     .......     |                      |     .......     |
		//          |      ret 0      |                      |      ret 0      |
		//          |      arg X      |                      |      arg X      |
		//          |     .......     |                      |     .......     |
		//          |      arg 1      |                      |      arg 1      |
		//          |      arg 0      |                      |      arg 0      |
		//          |   ReturnAddress |                      |   ReturnAddress |
		//          +-----------------+      ========>       +-----------------+
		//          |   ...........   |                      |   ...........   |
		//          |   spill slot M  |                      |   spill slot M  |
		//          |   ............  |                      |   ............  |
		//          |   spill slot 2  |                      |   spill slot 2  |
		//          |   spill slot 1  |                      |   spill slot 1  |
		//          |   clobbered 0   |               SP---> +-----------------+
		//          |   clobbered 1   |
		//          |   ...........   |
		//          |   clobbered N   |
		//   SP---> +-----------------+
		//             (low address)

		l := len(m.clobberedRegs) - 1
		for i := range m.clobberedRegs {
			vr := m.clobberedRegs[l-i] // reverse order to restore.
			load := m.allocateInstrAfterLowering()
			amode := addressModePreOrPostIndex(spVReg,
				16,    // stack pointer must be 16-byte aligned.
				false, // Increment after store.
			)
			// TODO: pair loads to reduce the number of instructions.
			switch regTypeToRegisterSizeInBits(vr.RegType()) {
			case 64: // save int reg.
				load.asULoad(operandNR(vr), amode, 64)
			case 128: // save vector reg.
				load.asFpuLoad(operandNR(vr), amode, 128)
			}
			cur.next = load
			load.prev = cur
			cur = load
		}
	}

	if s := m.spillSlotSize; s > 0 {
		// Adjust SP to the original value:
		//
		//            (high address)                        (high address)
		//          +-----------------+                  +-----------------+
		//          |     .......     |                  |     .......     |
		//          |      ret Y      |                  |      ret Y      |
		//          |     .......     |                  |     .......     |
		//          |      ret 0      |                  |      ret 0      |
		//          |      arg X      |                  |      arg X      |
		//          |     .......     |                  |     .......     |
		//          |      arg 1      |                  |      arg 1      |
		//          |      arg 0      |                  |      arg 0      |
		//          |   ReturnAddress |                  |   ReturnAddress |
		//          +-----------------+      ====>       +-----------------+ <---- SP
		//          |   ...........   |                     (low address)
		//          |   spill slot M  |
		//          |   ............  |
		//          |   spill slot 2  |
		//          |   spill slot 1  |
		//   SP---> +-----------------+
		//             (low address)
		//
		addSP := m.allocateAddOrSubStackPointer(spVReg, s, true, true)
		cur.next = addSP
		addSP.prev = cur
		cur = addSP
	}

	// Reload the return address (lr).
	//
	//            +-----------------+          +-----------------+
	//            |     .......     |          |     .......     |
	//            |      ret Y      |          |      ret Y      |
	//            |     .......     |          |     .......     |
	//            |      ret 0      |          |      ret 0      |
	//            |      arg X      |          |      arg X      |
	//            |     .......     |   ===>   |     .......     |
	//            |      arg 1      |          |      arg 1      |
	//            |      arg 0      |          |      arg 0      |
	//            |      xxxxx      |          +-----------------+ <---- SP
	//            |  ReturnAddress  |
	//    SP----> +-----------------+

	ldr := m.allocateInstrAfterLowering()
	amode := addressModePreOrPostIndex(spVReg, 16 /* stack pointer must be 16-byte aligned. */, false /* increment after loads */)
	ldr.asULoad(operandNR(lrVReg), amode, 64)
	cur.next = ldr
	ldr.prev = cur

	ldr.next = prevNext
	prevNext.prev = ldr
}

// saveRequiredRegs is the set of registers that must be saved/restored during growing stack when there's insufficient
// stack space left. Basically this is the combination of CalleeSavedRegisters plus argument registers execpt for x0,
// which always points to the execution context whenever the native code is entered from Go.
var saveRequiredRegs = []regalloc.VReg{
	x1VReg, x2VReg, x3VReg, x4VReg, x5VReg, x6VReg, x7VReg,
	x18VReg, x19VReg, x20VReg, x21VReg, x22VReg, x23VReg, x24VReg, x25VReg, x26VReg, x28VReg, lrVReg,
	v0VReg, v1VReg, v2VReg, v3VReg, v4VReg, v5VReg, v6VReg, v7VReg,
	v18VReg, v19VReg, v20VReg, v21VReg, v22VReg, v23VReg, v24VReg, v25VReg, v26VReg, v27VReg, v28VReg, v29VReg, v30VReg, v31VReg,
}

// insertStackBoundsCheck will insert the instructions after `cur` to check the
// stack bounds, and if there's no sufficient spaces required for the function,
// exit the execution and try growing it in Go world.
//
// TODO: we should be able to share the instructions across all the functions to reduce the size of compiled executable.
func (m *machine) insertStackBoundsCheck(requiredStackSize int64, cur *instruction) *instruction {
	if requiredStackSize%16 != 0 {
		panic("BUG")
	}

	if immm12op, ok := asImm12Operand(uint64(requiredStackSize)); ok {
		// sub tmp, sp, #requiredStackSize
		sub := m.allocateInstrAfterLowering()
		sub.asALU(aluOpSub, operandNR(tmpRegVReg), operandNR(spVReg), immm12op, true)
		sub.prev = cur
		cur.next = sub
		cur = sub
	} else {
		// This case, we first load the requiredStackSize into the temporary register,
		m.lowerConstantI64(tmpRegVReg, requiredStackSize)
		// lowerConstantI64 adds instructions into m.pendingInstructions,
		// so we manually link them together.
		for _, inserted := range m.pendingInstructions {
			cur.next = inserted
			inserted.prev = cur
			cur = inserted
		}
		m.pendingInstructions = m.pendingInstructions[:0]
		// Then subtract it.
		sub := m.allocateInstrAfterLowering()
		sub.asALU(aluOpSub, operandNR(tmpRegVReg), operandNR(spVReg), operandNR(tmpRegVReg), true)
		sub.prev = cur
		cur.next = sub
		cur = sub
	}

	tmp2 := x11VReg // Callee save, so it is safe to use it here in the prologue.

	// ldr tmp2, [executionContext #StackBottomPtr]
	ldr := m.allocateInstrAfterLowering()
	ldr.asULoad(operandNR(tmp2), addressMode{
		kind: addressModeKindRegUnsignedImm12,
		rn:   x0VReg, // execution context is always the first argument.
		imm:  wazevoapi.ExecutionContextOffsets.StackBottomPtr.I64(),
	}, 64)
	ldr.prev = cur
	cur.next = ldr
	cur = ldr

	// subs xzr, tmp, tmp2
	subs := m.allocateInstrAfterLowering()
	subs.asALU(aluOpSubS, operandNR(xzrVReg), operandNR(tmpRegVReg), operandNR(tmp2), true)
	subs.prev = cur
	cur.next = subs
	cur = subs

	// b.ge #imm
	cbr := m.allocateInstr()
	cbr.asCondBr(ge.asCond(), invalidLabel, false /* ignored */)
	cbr.prev = cur
	cur.next = cbr
	cur = cbr

	// Save the callee saved and argument registers.
	offset := wazevoapi.ExecutionContextOffsets.SavedRegistersBegin.I64()
	for _, v := range saveRequiredRegs {
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

	// Save the current stack pointer.
	cur = m.saveCurrentStackPointer(cur, x0VReg)

	// Set the exit status on the execution context.
	cur = m.setExitCode(cur, x0VReg, wazevoapi.ExitCodeGrowStack)

	// Set the required stack size and set it to the exec context.
	{
		// First load the requiredStackSize into the temporary register,
		m.lowerConstantI64(tmpRegVReg, requiredStackSize)
		// lowerConstantI64 adds instructions into m.pendingInstructions,
		// so we manually link them together.
		for _, inserted := range m.pendingInstructions {
			cur.next = inserted
			inserted.prev = cur
			cur = inserted
		}
		setRequiredStackSize := m.allocateInstrAfterLowering()
		setRequiredStackSize.asStore(operandNR(tmpRegVReg),
			addressMode{
				kind: addressModeKindRegUnsignedImm12,
				// Execution context is always the first argument.
				rn: x0VReg, imm: wazevoapi.ExecutionContextOffsets.StackGrowRequiredSize.I64(),
			}, 64)
		setRequiredStackSize.prev = cur
		cur.next = setRequiredStackSize
		cur = setRequiredStackSize
	}

	// Exit the execution.
	cur = m.storeReturnAddressAndExit(cur)

	// After the exit, restore the saved registers.
	offset = wazevoapi.ExecutionContextOffsets.SavedRegistersBegin.I64()
	for _, v := range saveRequiredRegs {
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

	// Now that we know the entire code, we can finalize how many bytes
	// we have to skip when the stack size is sufficient.
	var cbrOffset int64
	for _cur := cbr; ; _cur = _cur.next {
		cbrOffset += _cur.size()
		if _cur == cur {
			break
		}
	}
	cbr.condBrOffsetResolve(cbrOffset)
	return cur
}

func (m *machine) setExitCode(cur *instruction, execCtr regalloc.VReg, exitCode wazevoapi.ExitCode) *instruction {
	// Set the exit status on the execution context.
	// 	movz tmp, #wazevoapi.ExitCodeGrowStack
	// 	str tmp, [exec_context]
	loadStatusConst := m.allocateInstrAfterLowering()
	loadStatusConst.asMOVZ(tmpRegVReg, uint64(exitCode), 0, true)
	loadStatusConst.prev = cur
	cur.next = loadStatusConst
	cur = loadStatusConst
	setExistStatus := m.allocateInstrAfterLowering()
	setExistStatus.asStore(operandNR(tmpRegVReg),
		addressMode{
			kind: addressModeKindRegUnsignedImm12,
			rn:   execCtr, imm: wazevoapi.ExecutionContextOffsets.ExitCodeOffset.I64(),
		}, 32)
	setExistStatus.prev = cur
	cur.next = setExistStatus
	cur = setExistStatus
	return cur
}

func (m *machine) storeReturnAddressAndExit(cur *instruction) *instruction {
	// Read the return address into tmp, and store it in the execution context.
	adr := m.allocateInstrAfterLowering()
	adr.asAdr(tmpRegVReg, exitSequenceSize+8)
	adr.prev = cur
	cur.next = adr
	cur = adr
	storeReturnAddr := m.allocateInstrAfterLowering()
	storeReturnAddr.asStore(operandNR(tmpRegVReg),
		addressMode{
			kind: addressModeKindRegUnsignedImm12,
			// Execution context is always the first argument.
			rn: x0VReg, imm: wazevoapi.ExecutionContextOffsets.GoCallReturnAddress.I64(),
		}, 64)
	storeReturnAddr.prev = cur
	cur.next = storeReturnAddr
	cur = storeReturnAddr

	// Exit the execution.
	trapSeq := m.allocateInstrAfterLowering()
	trapSeq.asExitSequence(x0VReg)
	trapSeq.prev = cur
	cur.next = trapSeq
	cur = trapSeq
	return cur
}

func (m *machine) saveCurrentStackPointer(cur *instruction, execCtr regalloc.VReg) *instruction {
	// Save the current stack pointer:
	// 	mov tmp, sp,
	// 	str tmp, [exec_ctx, #stackPointerBeforeGoCall]
	movSp := m.allocateInstrAfterLowering()
	movSp.asMove64(tmpRegVReg, spVReg)
	movSp.prev = cur
	cur.next = movSp
	cur = movSp
	strSp := m.allocateInstrAfterLowering()
	strSp.asStore(operandNR(tmpRegVReg),
		addressMode{
			kind: addressModeKindRegUnsignedImm12,
			rn:   execCtr, imm: wazevoapi.ExecutionContextOffsets.StackPointerBeforeGrow.I64(),
		}, 64)
	strSp.prev = cur
	cur.next = strSp
	cur = strSp
	return cur
}
