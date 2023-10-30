// Package regalloc performs register allocation. The algorithm can work on any ISA by implementing the interfaces in
// api.go.
package regalloc

// References:
// * https://web.stanford.edu/class/archive/cs/cs143/cs143.1128/lectures/17/Slides17.pdf
// * https://en.wikipedia.org/wiki/Chaitin%27s_algorithm
// * https://llvm.org/ProjectsWithLLVM/2004-Fall-CS426-LS.pdf
// * https://pfalcon.github.io/ssabook/latest/book-full.pdf: Chapter 9. for liveness analysis.

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
)

// NewAllocator returns a new Allocator.
func NewAllocator(allocatableRegs *RegisterInfo) Allocator {
	a := Allocator{
		regInfo:       allocatableRegs,
		nodePool:      wazevoapi.NewPool[node](resetNode),
		blockInfoPool: wazevoapi.NewPool[blockInfo](resetBlockInfo),
	}
	for _, regs := range allocatableRegs.AllocatableRegisters {
		for _, r := range regs {
			a.allocatableSet[r] = true
		}
	}
	return a
}

type (
	// RegisterInfo holds the statically-known ISA-specific register information.
	RegisterInfo struct {
		// AllocatableRegisters is a 2D array of allocatable RealReg, indexed by regTypeNum and regNum.
		// The order matters: the first element is the most preferred one when allocating.
		AllocatableRegisters [NumRegType][]RealReg
		CalleeSavedRegisters [RealRegsNumMax]bool
		CallerSavedRegisters [RealRegsNumMax]bool
		RealRegToVReg        []VReg
		// RealRegName returns the name of the given RealReg for debugging.
		RealRegName func(r RealReg) string
		RealRegType func(r RealReg) RegType
	}

	// Allocator is a register allocator.
	Allocator struct {
		// regInfo is static per ABI/ISA, and is initialized by the machine during Machine.PrepareRegisterAllocator.
		regInfo *RegisterInfo
		// allocatableSet is a set of allocatable RealReg derived from regInfo. Static per ABI/ISA.
		allocatableSet [RealRegsNumMax]bool
		// allocatedRegSet is a set of RealReg that are allocated during the allocation phase. This is reset per function.
		allocatedRegSet          [RealRegsNumMax]bool
		allocatedCalleeSavedRegs []VReg
		nodePool                 wazevoapi.Pool[node]
		blockInfoPool            wazevoapi.Pool[blockInfo]
		// vRegIDToNode maps VRegID to the node whose node.v has the VRegID.
		vRegIDToNode [] /* VRegID to */ *node
		blockInfos   [] /* blockID to */ *blockInfo
		vs           []VReg
		spillHandler spillHandler
		// phis keeps track of the VRegs that are defined by phi functions.
		phiBlocks []Block
		phis      []VReg

		// Followings are re-used during various places e.g. coloring.
		realRegSet [RealRegsNumMax]bool
		realRegs   []RealReg
		nodes1     []*node
		nodes2     []*node
		nodes3     []*node
		dedup      []bool
	}

	// blockInfo is a per-block information used during the register allocation.
	blockInfo struct {
		liveOuts map[VReg]struct{}
		liveIns  map[VReg]struct{}
		defs     map[VReg]programCounter
		lastUses VRegTable
		// Pre-colored real registers can have multiple live ranges in one block.
		realRegUses [vRegIDReservedForRealNum][]programCounter
		realRegDefs [vRegIDReservedForRealNum][]programCounter
		intervalMng *intervalManager
	}

	// node represents a VReg.
	node struct {
		id     int
		v      VReg
		ranges []*interval
		// r is the real register assigned to this node. It is either a pre-colored register or a register assigned during allocation.
		r RealReg
		// neighbors are the nodes that this node interferes with. Such neighbors have the same RegType as this node.
		neighbors []*node
		// copyFromReal and copyToReal are the real registers that this node copies from/to. During the allocation phase,
		// we try to assign the same RealReg to copyFromReal and copyToReal so that we can remove the redundant copy.
		copyFromReal, copyToReal RealReg
		// copyFromVReg and copyToVReg are the same as above, but for VReg not backed by real registers.
		copyFromVReg, copyToVReg *node
		degree                   int
		visited                  bool
	}

	// programCounter represents an opaque index into the program which is used to represents a LiveInterval of a VReg.
	programCounter int32
)

// DoAllocation performs register allocation on the given Function.
func (a *Allocator) DoAllocation(f Function) {
	a.livenessAnalysis(f)
	a.buildLiveRanges(f)
	a.buildNeighbors()
	a.coloring()
	a.determineCalleeSavedRealRegs(f)
	a.assignRegisters(f)
	f.Done()
}

func (a *Allocator) determineCalleeSavedRealRegs(f Function) {
	a.allocatedCalleeSavedRegs = a.allocatedCalleeSavedRegs[:0]
	for i, allocated := range a.allocatedRegSet {
		if allocated {
			r := RealReg(i)
			if a.regInfo.isCalleeSaved(r) {
				a.allocatedCalleeSavedRegs = append(a.allocatedCalleeSavedRegs, a.regInfo.RealRegToVReg[r])
			}
		}
	}
	// In order to make the output deterministic, sort it now.
	sort.Slice(a.allocatedCalleeSavedRegs, func(i, j int) bool {
		return a.allocatedCalleeSavedRegs[i] < a.allocatedCalleeSavedRegs[j]
	})
	f.ClobberedRegisters(a.allocatedCalleeSavedRegs)
}

// We assign different pc to use and def in one instruction. That way we can, for example, use the same register in
// one instruction. E.g. add r0, r0, r0.
const (
	pcUseOffset = 0
	pcDefOffset = 1
	pcStride    = pcDefOffset + 1
)

// phiBlk returns the block that defines the given phi value, nil otherwise.
func (a *Allocator) phiBlk(id VRegID) Block {
	if int(id) >= len(a.phiBlocks) {
		return nil
	}
	return a.phiBlocks[id]
}

// liveAnalysis constructs Allocator.blockInfos.
// The algorithm here is described in https://pfalcon.github.io/ssabook/latest/book-full.pdf Chapter 9.2.
func (a *Allocator) livenessAnalysis(f Function) {
	// First, we need to allocate blockInfos.
	var maxBlockID int
	for blk := f.PostOrderBlockIteratorBegin(); blk != nil; blk = f.PostOrderBlockIteratorNext() { // Order doesn't matter.
		info := a.allocateBlockInfo(blk.ID())
		// If this is not the entry block, we should define phi nodes, which are not defined by instructions.
		for _, p := range blk.BlockParams() {
			info.defs[p] = 0 // Earliest definition is at the beginning of the block.
			a.phis = append(a.phis, p)
			pid := int(p.ID())
			if diff := pid + 1 - len(a.phiBlocks); diff > 0 {
				a.phiBlocks = append(a.phiBlocks, make([]Block, diff+1)...)
			}
			a.phiBlocks[pid] = blk
		}
		if blk.ID() > maxBlockID {
			maxBlockID = blk.ID()
		}
	}

	if maxBlockID >= len(a.dedup) {
		a.dedup = append(a.dedup, make([]bool, maxBlockID+1)...)
	}

	// Run the Algorithm 9.2 in the bool.
	for blk := f.PostOrderBlockIteratorBegin(); blk != nil; blk = f.PostOrderBlockIteratorNext() {
		blkID := blk.ID()
		info := a.allocateBlockInfo(blkID)

		ns := blk.Succs()
		for i := 0; i < ns; i++ {
			succ := blk.Succ(i)
			if succ == nil {
				continue
			}

			succID := succ.ID()
			if !a.dedup[succID] { // This means the back edge.
				continue
			}

			succInfo := a.blockInfoAt(succID)
			for v := range succInfo.liveIns {
				if a.phiBlk(v.ID()) != succ {
					info.liveOuts[v] = struct{}{}
					info.liveIns[v] = struct{}{}
				}
			}
		}

		var pc programCounter
		var minVRegID VRegIDMinSet
		for instr := blk.InstrIteratorBegin(); instr != nil; instr = blk.InstrIteratorNext() {
			uses := instr.Uses()
			for _, use := range uses {
				if !use.IsRealReg() {
					minVRegID.Observe(use)
				}
			}
			pc += pcStride
		}
		info.lastUses.Reset(minVRegID)

		for instr := blk.InstrRevIteratorBegin(); instr != nil; instr = blk.InstrRevIteratorNext() {
			pc -= pcStride
			var use, def VReg
			for _, def = range instr.Defs() {
				defID := def.ID()
				pos := pc + pcDefOffset
				if def.IsRealReg() {
					info.realRegDefs[defID] = append(info.realRegDefs[defID], pos)
				} else {
					info.defs[def] = pos
					delete(info.liveIns, def)
				}
			}
			for _, use = range instr.Uses() {
				pos := pc + pcUseOffset
				if use.IsRealReg() {
					id := use.ID()
					info.realRegUses[id] = append(info.realRegUses[id], pos)
				} else {
					if info.lastUses.Lookup(use) < 0 {
						info.lastUses.Insert(use, pos)
					}
					info.liveIns[use] = struct{}{}
				}
			}

			if instr.IsCopy() {
				a.recordCopyRelation(def, use)
			}

			// If the destination is a phi value, and ...
			if def.Valid() && a.phiBlk(def.ID()) != nil {
				if use.Valid() && use.IsRealReg() {
					// If the source is a real register, this is the beginning of the function, and
					// therefore we need to add the definition of the real register.
					r := use.ID()
					info.realRegDefs[r] = append(info.realRegDefs[r], 0)
				} else {
					// Otherwise, this is the definition of the phi value for the successor block.
					// So we need to make it outlive the block.
					info.liveOuts[def] = struct{}{}
				}
			}
		}
		a.dedup[blkID] = true
	}

	nrs := f.LoopNestingForestRoots()
	for i := 0; i < nrs; i++ {
		root := f.LoopNestingForestRoot(i)
		a.loopTreeDFS(root)
	}

	// Clears the dedup array for the next function.
	for i := 0; i <= maxBlockID; i++ {
		a.dedup[i] = false
	}
}

// loopTreeDFS implements the Algorithm 9.3 in the book.
// TODO: it should be easy to rewrite this as a non-recursive one.
func (a *Allocator) loopTreeDFS(root Block) {
	rootID := root.ID()
	info := a.blockInfoAt(rootID)
	tmp := map[VReg]struct{}{}
	for v := range info.liveIns {
		if a.phiBlk(v.ID()) != root {
			tmp[v] = struct{}{}
			info.liveOuts[v] = struct{}{}
		}
	}

	cn := root.LoopNestingForestChildren()
	for i := 0; i < cn; i++ {
		child := root.LoopNestingForestChild(i)
		childID := child.ID()
		childInfo := a.blockInfoAt(childID)
		for v := range tmp {
			childInfo.liveIns[v] = struct{}{}
			childInfo.liveOuts[v] = struct{}{}
		}
		if child.LoopHeader() {
			a.loopTreeDFS(child)
		}
	}
}

func (a *Allocator) buildLiveRanges(f Function) {
	for blk := f.PostOrderBlockIteratorBegin(); blk != nil; blk = f.PostOrderBlockIteratorNext() { // Order doesn't matter.
		blkID := blk.ID()
		info := a.blockInfoAt(blkID)
		a.buildLiveRangesForNonReals(info)
		a.buildLiveRangesForReals(info)
		info.intervalMng.build()
	}
}

func (a *Allocator) buildLiveRangesForNonReals(info *blockInfo) {
	ins, outs, defs := info.liveIns, info.liveOuts, info.defs

	// In order to do the deterministic allocation, we need to sort ins.
	vs := a.vs[:0]
	for v := range ins {
		vs = append(vs, v)
	}
	sort.SliceStable(vs, func(i, j int) bool {
		return vs[i].ID() < vs[j].ID()
	})
	for _, v := range vs {
		if v.IsRealReg() {
			panic("BUG: real registers should not be in liveIns")
		}
		var begin, end programCounter
		if _, ok := outs[v]; ok {
			// v is live-in and live-out, so it is live-through.
			begin, end = 0, math.MaxInt32
		} else {
			// v is killed at killPos.
			begin, end = 0, info.lastUses.Lookup(v)
		}
		n := a.getOrAllocateNode(v)
		intervalNode := info.intervalMng.insert(n, begin, end)
		n.ranges = append(n.ranges, intervalNode)
	}

	// In order to do the deterministic allocation, we need to sort defs.
	vs = vs[:0]
	for v := range defs {
		vs = append(vs, v)
	}
	sort.SliceStable(vs, func(i, j int) bool {
		return vs[i].ID() < vs[j].ID()
	})
	for _, v := range vs {
		defPos := defs[v]
		if v.IsRealReg() {
			panic("BUG: real registers should not be in defs")
		}

		if _, ok := ins[v]; ok {
			// This case, phi value is coming in (used somewhere) but re-defined at the end.
			continue
		}

		var end programCounter
		if _, ok := outs[v]; ok {
			// v is defined here and live-out, so it is live-through.
			end = math.MaxInt32
		} else {
			end = info.lastUses.Lookup(v)
			if end == -1 {
				// This case the defined value is not used at all.
				end = defPos
			}
		}
		n := a.getOrAllocateNode(v)
		intervalNode := info.intervalMng.insert(n, defPos, end)
		n.ranges = append(n.ranges, intervalNode)
	}

	// Reuse for the next block.
	a.vs = vs[:0]
}

// buildLiveRangesForReals builds live ranges for pre-colored real registers.
func (a *Allocator) buildLiveRangesForReals(info *blockInfo) {
	ds, us := info.realRegDefs, info.realRegUses

	for i := 0; i < RealRegsNumMax; i++ {
		r := RealReg(i)
		// Non allocation target registers are not needed here.
		if !a.allocatableSet[r] {
			continue
		}

		uses := us[r]
		defs := ds[r]
		if len(defs) != len(uses) {
			// This is likely a bug of the Instr interface implementation and/or ABI around call instructions.
			// E.g. call or ret instructions should specify that they use all the real registers (calling convention).
			panic(
				fmt.Sprintf(
					"BUG: real register (%s) is defined and used, but the number of defs and uses are different: %d (defs) != %d (uses)",
					a.regInfo.RealRegName(r), len(defs), len(uses),
				),
			)
		} else if len(uses) == 0 {
			continue
		}

		sort.Slice(uses, func(i, j int) bool {
			return uses[i] < uses[j]
		})
		sort.Slice(defs, func(i, j int) bool {
			return defs[i] < defs[j]
		})

		for i := range uses {
			n := a.allocateNode()
			n.r = r
			n.v = FromRealReg(r, a.regInfo.RealRegType(r))
			defined, used := defs[i], uses[i]
			intervalNode := info.intervalMng.insert(n, defined, used)
			n.ranges = append(n.ranges, intervalNode)
		}
	}
}

// Reset resets the allocator's internal state so that it can be reused.
func (a *Allocator) Reset() {
	a.nodePool.Reset()
	a.blockInfos = a.blockInfos[:0]
	a.blockInfoPool.Reset()
	for i := range a.vRegIDToNode {
		a.vRegIDToNode[i] = nil
	}
	for i := range a.allocatedRegSet {
		a.allocatedRegSet[i] = false
	}

	a.nodes1 = a.nodes1[:0]
	a.nodes2 = a.nodes2[:0]
	a.realRegs = a.realRegs[:0]
	for _, phi := range a.phis {
		a.phiBlocks[phi.ID()] = nil
	}
	a.phis = a.phis[:0]
	a.vs = a.vs[:0]
}

func (a *Allocator) allocateBlockInfo(blockID int) *blockInfo {
	if blockID >= len(a.blockInfos) {
		a.blockInfos = append(a.blockInfos, make([]*blockInfo, (blockID+1)-len(a.blockInfos))...)
	}
	info := a.blockInfos[blockID]
	if info == nil {
		info = a.blockInfoPool.Allocate()
		a.blockInfos[blockID] = info
	}
	return info
}

func (a *Allocator) blockInfoAt(blockID int) (info *blockInfo) {
	info = a.blockInfos[blockID]
	return
}

// getOrAllocateNode returns a node for the given virtual register.
// This assumes that VReg is not a real register-backed one, otherwise
// the lookup table vRegIDToNode will be overflowed.
func (a *Allocator) getOrAllocateNode(v VReg) (n *node) {
	if vid := int(v.ID()); vid < len(a.vRegIDToNode) {
		if n = a.vRegIDToNode[v.ID()]; n != nil {
			return
		}
	} else {
		a.vRegIDToNode = append(a.vRegIDToNode, make([]*node, vid+1)...)
	}
	n = a.allocateNode()
	n.r = RealRegInvalid
	n.v = v
	a.vRegIDToNode[v.ID()] = n
	return
}

func resetBlockInfo(i *blockInfo) {
	if i.intervalMng == nil {
		i.intervalMng = newIntervalManager()
	} else {
		i.intervalMng.reset()
	}
	i.liveOuts = resetMap(i.liveOuts)
	i.liveIns = resetMap(i.liveIns)
	i.defs = resetMap(i.defs)

	for index := range i.realRegUses {
		i.realRegUses[index] = i.realRegUses[index][:0]
		i.realRegDefs[index] = i.realRegDefs[index][:0]
	}
}

func resetNode(n *node) {
	n.r = RealRegInvalid
	n.v = VRegInvalid
	n.ranges = n.ranges[:0]
	n.neighbors = n.neighbors[:0]
	n.copyFromVReg = nil
	n.copyToVReg = nil
	n.copyFromReal = RealRegInvalid
	n.copyToReal = RealRegInvalid
	n.degree = 0
	n.visited = false
}

func resetMap[K comparable, V any](m map[K]V) map[K]V {
	if m == nil {
		m = make(map[K]V)
	} else {
		for v := range m {
			delete(m, v)
		}
	}
	return m
}

func (a *Allocator) allocateNode() (n *node) {
	n = a.nodePool.Allocate()
	n.id = a.nodePool.Allocated() - 1
	return
}

// Format is for debugging.
func (i *blockInfo) Format(ri *RegisterInfo) string {
	var buf strings.Builder
	buf.WriteString("\tliveOuts: ")
	for v := range i.liveOuts {
		buf.WriteString(fmt.Sprintf("%v ", v))
	}
	buf.WriteString("\n\tliveIns: ")
	for v := range i.liveIns {
		buf.WriteString(fmt.Sprintf("%v ", v))
	}
	buf.WriteString("\n\tdefs: ")
	for v, pos := range i.defs {
		buf.WriteString(fmt.Sprintf("%v@%v ", v, pos))
	}
	buf.WriteString("\n\tlastUses: ")
	i.lastUses.Range(func(v VReg, pos programCounter) {
		buf.WriteString(fmt.Sprintf("%v@%v ", v, pos))
	})
	buf.WriteString("\n\trealRegUses: ")
	for v, pos := range i.realRegUses {
		if len(pos) > 0 {
			buf.WriteString(fmt.Sprintf("%s@%v ", ri.RealRegName(RealReg(v)), pos))
		}
	}
	buf.WriteString("\n\trealRegDefs: ")
	for v, pos := range i.realRegDefs {
		if len(pos) > 0 {
			buf.WriteString(fmt.Sprintf("%s@%v ", ri.RealRegName(RealReg(v)), pos))
		}
	}
	return buf.String()
}

// String implements fmt.Stringer for debugging.
func (n *node) String() string {
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("v%v", n.v.ID()))
	if n.r != RealRegInvalid {
		buf.WriteString(fmt.Sprintf(":%v", n.r))
	}
	// Add neighbors
	buf.WriteString(" neighbors[")
	for _, n := range n.neighbors {
		buf.WriteString(fmt.Sprintf("v%v ", n.v.ID()))
	}
	buf.WriteString("]")
	return buf.String()
}

func (n *node) spill() bool {
	return n.r == RealRegInvalid
}

func (r *RegisterInfo) isCalleeSaved(reg RealReg) bool {
	return r.CalleeSavedRegisters[reg]
}

func (r *RegisterInfo) isCallerSaved(reg RealReg) bool {
	return r.CallerSavedRegisters[reg]
}

func (a *Allocator) recordCopyRelation(dst, src VReg) {
	sr, dr := src.IsRealReg(), dst.IsRealReg()
	switch {
	case sr && dr:
	case !sr && !dr:
		dstN := a.getOrAllocateNode(dst)
		srcN := a.getOrAllocateNode(src)
		dstN.copyFromVReg = srcN
		srcN.copyToVReg = dstN
	case sr && !dr:
		dstN := a.getOrAllocateNode(dst)
		dstN.copyFromReal = src.RealReg()
	case !sr && dr:
		srcN := a.getOrAllocateNode(src)
		srcN.copyToReal = dst.RealReg()
	}
}

// assignedRealReg returns either the assigned RealReg to this node or precolored RealReg.
func (n *node) assignedRealReg() RealReg {
	r := n.r
	if r != RealRegInvalid {
		return r
	}
	return n.v.RealReg()
}
