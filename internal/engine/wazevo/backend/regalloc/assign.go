package regalloc

import (
	"fmt"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
)

// assignRegisters assigns real registers to virtual registers on each instruction.
// This is called after coloring is done.
func (a *Allocator) assignRegisters(f Function) {
	for blk := f.ReversePostOrderBlockIteratorBegin(); blk != nil; blk = f.ReversePostOrderBlockIteratorNext() {
		info := a.blockInfoAt(blk.ID())
		lns := info.liveNodes
		a.assignRegistersPerBlock(f, blk, a.vRegIDToNode, lns)
	}
}

// assignRegistersPerBlock assigns real registers to virtual registers on each instruction in a block.
func (a *Allocator) assignRegistersPerBlock(f Function, blk Block, vRegIDToNode []*node, liveNodes []liveNodeInBlock) {
	if wazevoapi.RegAllocLoggingEnabled {
		fmt.Println("---------------------- assigning registers for block", blk.ID(), "----------------------")
	}

	var pc programCounter
	for instr := blk.InstrIteratorBegin(); instr != nil; instr = blk.InstrIteratorNext() {
		a.assignRegistersPerInstr(f, pc, instr, vRegIDToNode, liveNodes)
		pc += pcStride
	}
}

func (a *Allocator) assignRegistersPerInstr(f Function, pc programCounter, instr Instr, vRegIDToNode []*node, liveNodes []liveNodeInBlock) {
	if wazevoapi.RegAllocValidationEnabled {
		// Check if the liveNodes are sorted by the start program counter.
		for i := 1; i < len(liveNodes); i++ {
			n, m := liveNodes[i-1], liveNodes[i]
			if n.n.ranges[n.rangeIndex].begin > m.n.ranges[m.rangeIndex].begin {
				panic(fmt.Sprintf("BUG: liveNodes are not sorted by the start program counter: %d > %d",
					n.n.ranges[n.rangeIndex].begin, m.n.ranges[m.rangeIndex].begin,
				))
			}
		}
	}

	if indirect := instr.IsIndirectCall(); instr.IsCall() || indirect {
		// Only take care of non-real VRegs (e.g. VReg.IsRealReg() == false) since
		// the real VRegs are already placed in the right registers at this point.
		a.collectActiveNonRealVRegsAt(
			// To find the all the live registers "after" call, we need to add pcDefOffset for search.
			pc+pcDefOffset,
			liveNodes)
		for _, active := range a.nodes1 {
			if r := active.r; a.regInfo.isCallerSaved(r) {
				v := active.v.SetRealReg(r)
				f.StoreRegisterBefore(v, instr)
				f.ReloadRegisterAfter(v, instr)
			}
		}
		if indirect {
			// Direct function calls do not need assignment, while indirect one needs the assignment on the function pointer.
			a.assignIndirectCall(f, instr, vRegIDToNode)
		}

		if wazevoapi.RegAllocValidationEnabled {
			for _, def := range instr.Defs() {
				if !def.IsRealReg() {
					panic(fmt.Sprintf("BUG: call/indirect call instruction must define only real registers: %s", def))
				}
			}
		}
		return
	} else if instr.IsReturn() {
		return
	}

	usesSpills := a.vs[:0]
	uses := instr.Uses()
	for i, u := range uses {
		if u.IsRealReg() {
			a.vs = append(a.vs, u)
			continue
		}
		if wazevoapi.RegAllocLoggingEnabled {
			fmt.Printf("%s uses %d\n", instr, u.ID())
		}
		n := vRegIDToNode[u.ID()]
		if !n.spill() {
			instr.AssignUse(i, u.SetRealReg(n.r))
		} else {
			usesSpills = append(usesSpills, u)
		}
	}

	defs := instr.Defs()
	defSpill := VRegInvalid
	switch len(defs) {
	case 0:
	case 1:
		d := defs[0]
		if !d.IsRealReg() {
			if wazevoapi.RegAllocLoggingEnabled {
				fmt.Printf("%s defines %d\n", instr, d.ID())
			}

			n := vRegIDToNode[d.ID()]
			if !n.spill() {
				instr.AssignDef(d.SetRealReg(n.r))
			} else {
				defSpill = n.v
			}
		}
	default:
		// multiple defs (== call instruction) are special cased, and no need to assign (already real regs following the calling convention.
	}

	a.handleSpills(f, pc, instr, vRegIDToNode, liveNodes, usesSpills, defSpill)
	a.vs = usesSpills[:0] // for reuse.
}

func (a *Allocator) handleSpills(
	f Function, pc programCounter, instr Instr, vRegIDToNode []*node, liveNodes []liveNodeInBlock,
	usesSpills []VReg, defSpill VReg,
) {
	if len(usesSpills) == 0 && !defSpill.Valid() {
		return
	}
	panic("TODO")
}

func (a *Allocator) assignIndirectCall(f Function, instr Instr, vRegIDToNode []*node) {
	a.nodes1 = a.nodes1[:0]
	uses := instr.Uses()
	if wazevoapi.RegAllocValidationEnabled {
		var nonRealRegs int
		for _, u := range uses {
			if !u.IsRealReg() {
				nonRealRegs++
			}
		}
		if nonRealRegs != 1 {
			panic(fmt.Sprintf("BUG: indirect call must have only one non-real register (for function pointer): %d", nonRealRegs))
		}
	}

	var v VReg
	for _, u := range uses {
		if !u.IsRealReg() {
			v = u
			break
		}
	}

	if v.RegType() != RegTypeInt {
		panic(fmt.Sprintf("BUG: function pointer for indirect call must be an integer register: %s", v))
	}

	n := vRegIDToNode[v.ID()]
	if n.spill() {
		// If the function pointer is spilled, we need to reload it to a register.
		// But at this point, all the caller-saved registers are saved, we can use a callee-saved register to reload.
		for _, r := range a.regInfo.AllocatableRegisters[RegTypeInt] {
			if a.regInfo.isCallerSaved(r) {
				v = v.SetRealReg(r)
				f.ReloadRegisterBefore(v, instr)
				break
			}
		}
	} else {
		v = v.SetRealReg(n.r)
	}
	instr.AssignUse(0, v)
}

// collectActiveNonRealVRegsAt collects the set of active registers at the given program counter into `a.nodes1` slice by appending
// the found registers from its beginning. This excludes the VRegs backed by a real register since this is used to list the registers
// alive but not used by a call instruction.
func (a *Allocator) collectActiveNonRealVRegsAt(pc programCounter, liveNodes []liveNodeInBlock) {
	nodes := a.nodes1[:0]
	for _, live := range liveNodes {
		n := live.n
		if n.spill() || n.v.IsRealReg() {
			continue
		}
		r := &n.ranges[live.rangeIndex]
		if r.begin > pc {
			// liveNodes are sorted by the start program counter, so we can break here.
			break
		}
		if pc <= r.end { // pc is in the range.
			nodes = append(nodes, n)
		}
	}
	a.nodes1 = nodes
}
