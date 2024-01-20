package amd64

import "github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"

// CompileStackGrowCallSequence implements backend.Machine.
func (m *machine) CompileStackGrowCallSequence() []byte {
	// TODO
	ud2 := m.allocateInstr().asUD2()
	m.encodeWithoutRelResolution(ud2)
	return m.c.Buf()
}

// SetupPrologue implements backend.Machine.
func (m *machine) SetupPrologue() {
	cur := m.ectx.RootInstr
	prevInitInst := cur.next

	// At this point, we have the stack layout as follows:
	//
	//                   (high address)
	//                 +-----------------+ <----- RBP (somewhere in the middle of the stack)
	//                 |     .......     |
	//                 |      ret Y      |
	//                 |     .......     |
	//                 |      ret 0      |
	//                 |      arg X      |
	//                 |     .......     |
	//                 |      arg 1      |
	//                 |      arg 0      |
	//                 |   Return Addr   |
	//       RSP ----> +-----------------+
	//                    (low address)

	// First, we push the RBP, and update the RBP to the current RSP.
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
	// 		push %rbp
	// 		mov %rsp, %rbp
	cur = linkInstr(cur, m.allocateInstr().asPush64(newOperandReg(rbpVReg)))
	cur = linkInstr(cur, m.allocateInstr().asMovRR(rspVReg, rbpVReg, true))

	if !m.stackBoundsCheckDisabled { //nolint
		// TODO: stack bounds check
	}

	//
	//            (high address)
	//          +-----------------+                  +-----------------+
	//          |     .......     |                  |     .......     |
	//          |      ret Y      |                  |      ret Y      |
	//          |     .......     |                  |     .......     |
	//          |      ret 0      |                  |      ret 0      |
	//          |      arg X      |                  |      arg X      |
	//          |     .......     |                  |     .......     |
	//          |      arg 1      |                  |      arg 1      |
	//          |      arg 0      |                  |      arg 0      |
	//          |      xxxxx      |                  |      xxxxx      |
	//          |   Return Addr   |                  |   Return Addr   |
	//          |    Caller_RBP   |      ====>       |    Caller_RBP   |
	// RBP,RSP->+-----------------+                  +-----------------+ <----- RBP
	//             (low address)                     |   clobbered M   |
	//                                               |   clobbered 1   |
	//                                               |   ...........   |
	//                                               |   clobbered 0   |
	//                                               +-----------------+ <----- RSP
	//
	if regs := m.clobberedRegs; len(regs) > 0 {
		for i := range regs {
			r := regs[len(regs)-1-i] // Reverse order.
			if r.RegType() == regalloc.RegTypeInt {
				cur = linkInstr(cur, m.allocateInstr().asPush64(newOperandReg(r)))
			} else {
				// Push the XMM register is not supported by the PUSH instruction.
				spDec := m.allocateInstr().asAluRmiR(aluRmiROpcodeSub, newOperandImm32(uint32(16)), rspVReg, true)
				cur = linkInstr(cur, spDec)
				push := m.allocateInstr().asXmmMovRM(
					sseOpcodeMovdqu, r, newOperandMem(newAmodeImmReg(0, rspVReg)),
				)
				cur = linkInstr(cur, push)
			}
		}
	}

	if size := m.spillSlotSize; size > 0 {
		panic("TODO: allocate spill slots")
	}

	linkInstr(cur, prevInitInst)
}

// SetupEpilogue implements backend.Machine.
func (m *machine) SetupEpilogue() {
	ectx := m.ectx
	for cur := ectx.RootInstr; cur != nil; cur = cur.next {
		if cur.kind == ret {
			m.setupEpilogueAfter(cur.prev)
			continue
		}

		// Removes the redundant copy instruction.
		// TODO: doing this in `SetupEpilogue` seems weird. Find a better home.
		if cur.IsCopy() && cur.op1.r.RealReg() == cur.op2.r.RealReg() {
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

	// At this point, we have the stack layout as follows:
	//
	//            (high address)
	//          +-----------------+
	//          |     .......     |
	//          |      ret Y      |
	//          |     .......     |
	//          |      ret 0      |
	//          |      arg X      |
	//          |     .......     |
	//          |      arg 1      |
	//          |      arg 0      |
	//          |   ReturnAddress |
	//          |   Caller_RBP    |
	//          +-----------------+ <--- RBP
	//          |    clobbered M  |
	//          |   ............  |
	//          |    clobbered 1  |
	//          |    clobbered 0  |
	//          |   spill slot N  |
	//          |   ............  |
	//          |   spill slot 0  |
	//          +-----------------+ <--- RSP
	//             (low address)

	if size := m.spillSlotSize; size > 0 {
		panic("TODO: deallocate spill slots")
	}

	//
	//             (high address)
	//            +-----------------+                     +-----------------+
	//            |     .......     |                     |     .......     |
	//            |      ret Y      |                     |      ret Y      |
	//            |     .......     |                     |     .......     |
	//            |      ret 0      |                     |      ret 0      |
	//            |      arg X      |                     |      arg X      |
	//            |     .......     |                     |     .......     |
	//            |      arg 1      |                     |      arg 1      |
	//            |      arg 0      |                     |      arg 0      |
	//            |   ReturnAddress |                     |   ReturnAddress |
	//            |    Caller_RBP   |                     |    Caller_RBP   |
	//   RBP ---> +-----------------+      ========>      +-----------------+ <---- RSP, RBP
	//            |    clobbered M  |
	//            |   ............  |
	//            |    clobbered 1  |
	//            |    clobbered 0  |
	//   RSP ---> +-----------------+
	//               (low address)
	//
	if regs := m.clobberedRegs; len(regs) > 0 {
		for _, r := range regs {
			if r.RegType() == regalloc.RegTypeInt {
				cur = linkInstr(cur, m.allocateInstr().asPop64(r))
			} else {
				// Pop the XMM register is not supported by the POP instruction.
				pop := m.allocateInstr().asXmmUnaryRmR(
					sseOpcodeMovdqu, newOperandMem(newAmodeImmReg(0, rspVReg)), r,
				)
				cur = linkInstr(cur, pop)
				spInc := m.allocateInstr().asAluRmiR(
					aluRmiROpcodeAdd, newOperandImm32(uint32(16)), rspVReg, true,
				)
				cur = linkInstr(cur, spInc)
			}
		}
	}

	// Now roll back the RBP to the original, which results in RBP being the return address.
	// 		pop  %rbp
	cur = linkInstr(cur, m.allocateInstr().asPop64(rbpVReg))

	linkInstr(cur, prevNext)
}
