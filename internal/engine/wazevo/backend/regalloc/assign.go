package regalloc

import (
	"fmt"
	"sort"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
)

// assignRegisters assigns real registers to virtual registers on each instruction.
// This is called after coloring is done.
func (a *Allocator) assignRegisters(f Function) {
	for blk := f.ReversePostOrderBlockIteratorBegin(); blk != nil; blk = f.ReversePostOrderBlockIteratorNext() {
		a.assignRegistersPerBlock(f, blk)
	}
}

// assignRegistersPerBlock assigns real registers to virtual registers on each instruction in a block.
func (a *Allocator) assignRegistersPerBlock(f Function, blk Block) {
	if wazevoapi.RegAllocLoggingEnabled {
		fmt.Println("---------------------- assigning registers for block", blk.ID(), "----------------------")
	}

	info := a.blockInfoAt(blk.ID())
	a.aliveSet = resetMap(a.aliveSet)
	for v := range info.liveIns {
		n := a.getOrAllocateNode(v)
		a.aliveSet[n] = struct{}{}
	}

	if !blk.Entry() {
		for _, arg := range blk.BlockParams() {
			n := a.getOrAllocateNode(arg)
			a.aliveSet[n] = struct{}{}
		}
	}

	var pc programCounter
	for instr := blk.InstrIteratorBegin(); instr != nil; instr = blk.InstrIteratorNext() {
		if wazevoapi.RegAllocLoggingEnabled {
			fmt.Printf("--- handling %v ---\n", instr)
			for alive := range a.aliveSet {
				fmt.Println("\t", alive)
			}
		}

		a.assignRegistersPerInstr(f, info, pc, instr)
		pc += pcStride
	}
}

func (a *Allocator) collectOrderedActiveNodes(real bool) {
	a.nodes1 = a.nodes1[:0]
	for n := range a.aliveSet {
		if real {
			if n.assignedRealReg() == RealRegInvalid {
				continue
			}
		} else {
			if n.spill() || n.v.IsRealReg() {
				continue
			}
		}
		a.nodes1 = append(a.nodes1, n)
	}
	sort.Slice(a.nodes1, func(i, j int) bool {
		return a.nodes1[i].v.ID() < a.nodes1[j].v.ID()
	})
}

func (a *Allocator) updateAliveNodesByUse(info *blockInfo, pc programCounter, instr Instr) {
	for _, use := range instr.Uses() {
		n := a.vRegIDToNode[use.ID()]
		v := n.v
		if v.IsRealReg() {
			delete(a.aliveSet, n)
		} else {
			if info.lastUses.Lookup(v) == pc {
				if _, ok := info.liveOuts[v]; !ok {
					delete(a.aliveSet, n)
				}
			}
		}
	}
}

func (a *Allocator) updateAliveNodesByDef(info *blockInfo, instr Instr) {
	for _, def := range instr.Defs() {
		n := a.vRegIDToNode[def.ID()]
		v := n.v
		if !v.IsRealReg() && info.lastUses.Lookup(v) < 0 {
			if _, ok := info.liveOuts[v]; !ok {
				continue
			}
		}
		a.aliveSet[n] = struct{}{}
	}
}

func (a *Allocator) assignRegistersPerInstr(f Function, info *blockInfo, pc programCounter, instr Instr) {
	if indirect := instr.IsIndirectCall(); instr.IsCall() || indirect {
		a.updateAliveNodesByUse(info, pc, instr)
		a.updateAliveNodesByDef(info, instr)
		a.collectOrderedActiveNodes(false)
		for _, active := range a.nodes1 {
			if r := active.r; a.regInfo.isCallerSaved(r) {
				v := active.v.SetRealReg(r)
				f.StoreRegisterBefore(v, instr)
				f.ReloadRegisterAfter(v, instr)
			}
		}
		if indirect {
			// Direct function calls do not need assignment, while indirect one needs the assignment on the function pointer.
			a.assignIndirectCall(f, instr)
		}
		return
	} else if instr.IsReturn() {
		return
	}

	usesSpills := a.vs[:0]
	uses := instr.Uses()
	for i, u := range uses {
		if u.IsRealReg() {
			continue
		}
		if wazevoapi.RegAllocLoggingEnabled {
			fmt.Printf("%s uses %s(%d)\n", instr, u.RegType(), u.ID())
		}
		n := a.vRegIDToNode[u.ID()]
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
				fmt.Printf("%s defines %s(%d)\n", instr, d.RegType(), d.ID())
			}

			n := a.vRegIDToNode[d.ID()]
			if !n.spill() {
				instr.AssignDef(d.SetRealReg(n.r))
			} else {
				defSpill = n.v
			}
		}
	default:
		panic("BUG: multiple def instructions must be special cased")
	}

	a.handleSpills(f, info, pc, instr, usesSpills, defSpill)
	a.vs = usesSpills[:0] // for reuse.
}

func (a *Allocator) handleSpills(
	f Function, info *blockInfo, pc programCounter, instr Instr,
	usesSpills []VReg, defSpill VReg,
) {
	_usesSpills, _defSpill := len(usesSpills) > 0, defSpill.Valid()
	switch {
	case !_usesSpills && !_defSpill: // Nothing to do.
		a.updateAliveNodesByUse(info, pc, instr)
		a.updateAliveNodesByDef(info, instr)
	case !_usesSpills && _defSpill: // Only definition is spilled.
		a.updateAliveNodesByUse(info, pc, instr)
		a.updateAliveNodesByDef(info, instr)
		a.collectOrderedActiveNodes(true)
		a.spillHandler.init(a.nodes1, instr)

		r, evictedNode := a.spillHandler.getUnusedOrEvictReg(defSpill.RegType(), a.regInfo)
		if evictedNode != nil {
			evictedNodeV := evictedNode.v.SetRealReg(evictedNode.assignedRealReg())
			f.StoreRegisterBefore(evictedNodeV, instr)
			f.ReloadRegisterAfter(evictedNodeV, instr)
		}

		defSpill = defSpill.SetRealReg(r)
		instr.AssignDef(defSpill)

		f.StoreRegisterAfter(defSpill, instr)

	case _usesSpills:
		a.updateAliveNodesByUse(info, pc, instr)
		a.collectOrderedActiveNodes(true)
		a.spillHandler.init(a.nodes1, instr)

		var evicted [3]*node
		var evictedCount int
		for i, u := range usesSpills {
			r, evictedNode := a.spillHandler.getUnusedOrEvictReg(u.RegType(), a.regInfo)
			if evictedNode != nil {
				evicted[evictedCount] = evictedNode
				evictedCount++
			}
			usesSpills[i] = u.SetRealReg(r)
		}

		for i := 0; i < evictedCount; i++ {
			evictedNode := evicted[i]
			evictedNodeV := evictedNode.v.SetRealReg(evictedNode.assignedRealReg())
			f.StoreRegisterBefore(evictedNodeV, instr)
			f.ReloadRegisterAfter(evictedNodeV, instr)
		}

		for _, u := range usesSpills {
			f.ReloadRegisterBefore(u, instr)
		}

		for useIndex, v := range instr.Uses() {
			for _, u := range usesSpills {
				if v.ID() == u.ID() {
					instr.AssignUse(useIndex, u)
				}
			}
		}

		if _defSpill {
			// We can reuse the register in usesSpills for the definition.
			for _, u := range usesSpills {
				if defSpill.RegType() == u.RegType() {
					defSpill = defSpill.SetRealReg(u.RealReg())
					break
				}
			}

			if !defSpill.IsRealReg() {
				// This case, the destination register type is different from the source registers.
				a.updateAliveNodesByDef(info, instr)
				a.collectOrderedActiveNodes(true)
				a.spillHandler.init(a.nodes1, instr)
				r, evictedNode := a.spillHandler.getUnusedOrEvictReg(defSpill.RegType(), a.regInfo)
				if evictedNode != nil {
					evictedNodeV := evictedNode.v.SetRealReg(evictedNode.assignedRealReg())
					f.StoreRegisterBefore(evictedNodeV, instr)
					f.ReloadRegisterAfter(evictedNodeV, instr)
				}
				defSpill = defSpill.SetRealReg(r)
			}

			instr.AssignDef(defSpill)
			f.StoreRegisterAfter(defSpill, instr)
		}
	}
}

func (a *Allocator) assignIndirectCall(f Function, instr Instr) {
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

	n := a.vRegIDToNode[v.ID()]
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
