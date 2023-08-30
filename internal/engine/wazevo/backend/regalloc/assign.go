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

	if direct := instr.IsCall(); direct || instr.IsIndirectCall() {
		// Only take care of non-real VRegs (e.g. VReg.IsRealReg() == false) since
		// the real VRegs are already placed in the right registers at this point.
		a.nodes2 = a.collectActiveNonRealVRegsAt(a.nodes2,
			// To find the all the live registers "after" call, we need to add pcDefOffset for search.
			pc+pcDefOffset,
			liveNodes)
		for _, active := range a.nodes2 {
			if r := active.r; a.regInfo.isCallerSaved(r) {
				v := active.v.SetRealReg(r)
				f.StoreRegisterBefore(v, instr)
				f.ReloadRegisterAfter(v, instr)
			}
		}
		// Direct function calls do not need assignment, while indirect one needs the assignment on the function pointer.
		if direct {
			return
		}
	} else if instr.IsReturn() {
		return
	}

	a.nodes1 = a.nodes1[:0]
	a.vs = a.vs[:0]
	uses := instr.Uses()
	for _, u := range uses {
		if u.IsRealReg() {
			a.vs = append(a.vs, u)
			continue
		}
		if wazevoapi.RegAllocLoggingEnabled {
			fmt.Printf("%s uses %d\n", instr, u.ID())
		}
		n := vRegIDToNode[u.ID()]
		if !n.spill() {
			a.vs = append(a.vs, u.SetRealReg(n.r))
		} else {
			a.nodes1 = append(a.nodes1, n)
		}
	}

	if len(a.nodes1) == 0 { // no spill.
		instr.AssignUses(a.vs)
	} else {
		panic("TODO: handle spills.")
	}

	defs := instr.Defs()
	switch len(defs) {
	case 0:
		return
	case 1:
	default:
		// multiple defs (== call instruction) can be special cased, and no need to assign (already real regs following the calling convention.
		return
	}

	d := defs[0]
	if d.IsRealReg() {
		return
	}
	if wazevoapi.RegAllocLoggingEnabled {
		fmt.Printf("%s defines %d\n", instr, d.ID())
	}

	n := vRegIDToNode[d.ID()]
	if !n.spill() {
		instr.AssignDef(d.SetRealReg(n.r))
	} else {
		panic("TODO: handle spills.")
	}
}

// collectActiveNonRealVRegsAt collects the set of active registers at the given program counter into `nodes` slice by appending
// the found registers from its beginning. This excludes the VRegs backed by a real register since this is used to list the registers
// alive but not used by a call instruction.
func (a *Allocator) collectActiveNonRealVRegsAt(nodes []*node, pc programCounter, liveNodes []liveNodeInBlock) []*node {
	nodes = nodes[:0]
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
	return nodes
}
