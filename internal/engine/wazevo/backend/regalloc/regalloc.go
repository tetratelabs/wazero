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
		regInfo:         allocatableRegs,
		nodePool:        wazevoapi.NewPool[node](),
		realRegSet:      make(map[RealReg]struct{}),
		nodeSet:         make(map[*node]int),
		allocatedRegSet: make(map[RealReg]struct{}),
	}
	allocatableSet := make(map[RealReg]struct{},
		len(allocatableRegs.AllocatableRegisters[RegTypeInt])+len(allocatableRegs.AllocatableRegisters[RegTypeFloat]),
	)
	for _, regs := range allocatableRegs.AllocatableRegisters {
		for _, r := range regs {
			allocatableSet[r] = struct{}{}
		}
	}
	a.allocatableSet = allocatableSet
	return a
}

type (
	// RegisterInfo holds the statically-known ISA-specific register information.
	RegisterInfo struct {
		// AllocatableRegisters is a 2D array of allocatable RealReg, indexed by regTypeNum and regNum.
		// The order matters: the first element is the most preferred one when allocating.
		AllocatableRegisters [RegTypeNum][]RealReg
		CalleeSavedRegisters map[RealReg]struct{}
		CallerSavedRegisters map[RealReg]struct{}
		RealRegToVReg        []VReg
		// RealRegName returns the name of the given RealReg for debugging.
		RealRegName func(r RealReg) string
	}

	// Allocator is a register allocator.
	Allocator struct {
		// regInfo is static per ABI/ISA, and is initialized by the machine during Machine.PrepareRegisterAllocator.
		regInfo                  *RegisterInfo
		allocatableSet           map[RealReg]struct{}
		allocatedRegSet          map[RealReg]struct{}
		allocatedCalleeSavedRegs []VReg
		nodePool                 wazevoapi.Pool[node]
		// vRegIDToNode maps VRegID to the node whose node.v has the VRegID.
		vRegIDToNode [] /* VRegID to */ *node
		blockInfos   [] /* blockID to */ blockInfo
		vs           []VReg

		// Followings are re-used during coloring and activeRegistersAt
		realRegSet map[RealReg]struct{}
		realRegs   []RealReg
		nodeSet    map[*node]int
		nodes1     []*node
		nodes2     []*node
	}

	// blockInfo is a per-block information used during the register allocation.
	blockInfo struct {
		// TODO: reuse!!!
		liveOuts map[VReg]struct{}
		liveIns  map[VReg]struct{}
		defs     map[VReg]programCounter
		lastUses map[VReg]programCounter
		kills    map[VReg]programCounter
		// Pre-colored real registers can have multiple live ranges in one block.
		realRegUses map[VReg][]programCounter
		realRegDefs map[VReg][]programCounter
		liveNodes   []liveNodeInBlock
	}

	liveNodeInBlock struct {
		// rangeIndex is the index to n.ranges which represents the live range of n.v in the block.
		rangeIndex int
		n          *node
	}

	// node represents a node interference graph of LiveRange(s) of VReg(s).
	node struct {
		v VReg
		// ranges holds the live ranges of this node per block. This will be accessed by
		// liveNodeInBlock.rangeIndex, which in turn is stored in blockInfo.liveNodes.
		ranges []liveRange
		// r is the real register assigned to this node. It is either a pre-colored register or a register assigned during allocation.
		r RealReg
		// neighbors are the nodes that this node interferes with. Such neighbors have the same RegType as this node.
		neighbors map[*node]struct{}
		// copyFromReal and copyToReal are the real registers that this node copies from/to. During the allocation phase,
		// we try to assign the same RealReg to copyFromReal and copyToReal so that we can remove the redundant copy.
		copyFromReal, copyToReal RealReg
		// copyFromVReg and copyToVReg are the same as above, but for VReg not backed by real registers.
		copyFromVReg, copyToVReg *node
	}

	// liveRange represents a lifetime of a VReg. Both begin (LiveInterval[0]) and end (LiveInterval[1]) are inclusive.
	liveRange struct {
		blockID    int
		begin, end programCounter
	}

	// programCounter represents an opaque index into the program which is used to represents a LiveInterval of a VReg.
	programCounter int64
)

// DoAllocation performs register allocation on the given Function.
func (a *Allocator) DoAllocation(f Function) {
	a.livenessAnalysis(f)
	a.buildLiveRanges(f)
	a.buildNeighbors(f)
	a.coloring()
	a.determineCalleeSavedRealRegs(f)
	a.assignRegisters(f)
	f.Done()
}

func (a *Allocator) determineCalleeSavedRealRegs(f Function) {
	a.allocatedCalleeSavedRegs = a.allocatedCalleeSavedRegs[:0]
	for r := range a.allocatedRegSet {
		if a.regInfo.isCalleeSaved(r) {
			a.allocatedCalleeSavedRegs = append(a.allocatedCalleeSavedRegs, a.regInfo.RealRegToVReg[r])
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

// liveAnalysis constructs Allocator.blockInfos.
// The algorithm here is described in https://pfalcon.github.io/ssabook/latest/book-full.pdf Chapter 9.4.
//
// TODO: this might not be efficient. We should be able to leverage dominance tree, etc.
func (a *Allocator) livenessAnalysis(f Function) {
	// First, we need to allocate blockInfos.
	for blk := f.PostOrderBlockIteratorBegin(); blk != nil; blk = f.PostOrderBlockIteratorNext() { // Order doesn't matter.
		a.allocateBlockInfo(blk.ID())
	}

	// Gathers all defs, lastUses, and VRegs in use (into a.vs).
	a.vs = a.vs[:0]
	for blk := f.PostOrderBlockIteratorBegin(); blk != nil; blk = f.PostOrderBlockIteratorNext() {
		blkID := blk.ID()
		info := a.blockInfoAt(blkID)

		var pc programCounter
		for instr := blk.InstrIteratorBegin(); instr != nil; instr = blk.InstrIteratorNext() {
			var srcVR, dstVR VReg
			for _, use := range instr.Uses() {
				srcVR = use
				pos := pc + pcUseOffset
				if use.IsRealReg() {
					info.addRealRegUsage(use, pos)
				} else {
					info.lastUses[use] = pos
				}
			}
			for _, def := range instr.Defs() {
				dstVR = def
				pos := pc + pcDefOffset
				if def.IsRealReg() {
					info.realRegDefs[def] = append(info.realRegDefs[def], pos)
				} else {
					if _, ok := info.defs[def]; !ok {
						// This means that this VReg is defined multiple times in a series of instructions
						// e.g. loading arbitrary constant in arm64, and we only need the earliest
						// definition to construct live range.
						info.defs[def] = pos
					}

					a.vs = append(a.vs, def)
				}
			}
			if instr.IsCopy() {
				a.recordCopyRelation(dstVR, srcVR)
			}
			pc += pcStride
		}

		if wazevoapi.RegAllocLoggingEnabled {
			fmt.Printf("constructed block info for block[%d]:\n%s\n\n", blkID, info)
		}
	}

	// Run the Algorithm 9.9. in the book. This will construct blockInfo.liveIns and blockInfo.liveOuts.
	// Note that we don't have "phi"s at this point, but rather they are lowered to special VRegs which
	// have multiple defs.
	for _, v := range a.vs {
		if v.IsRealReg() {
			// Real registers do not need to be tracked in liveOuts and liveIns because they are not allocation targets.
			panic("BUG")
		}
		for blk := f.PostOrderBlockIteratorBegin(); blk != nil; blk = f.PostOrderBlockIteratorNext() {
			if len(blk.Preds()) == 0 && !blk.Entry() {
				panic(fmt.Sprintf("block without predecessor must be optimized out by the compiler: %d", blk.ID()))
			}
			info := a.blockInfoAt(blk.ID())
			if _, ok := info.lastUses[v]; !ok {
				continue
			}
			// TODO: we might want to avoid recursion here.
			a.upAndMarkStack(blk, v, 0)
		}
	}

	// Now that we finished gathering liveIns, liveOuts, defs, and lastUses, the only thing left is to construct kills.
	for blk := f.PostOrderBlockIteratorBegin(); blk != nil; blk = f.PostOrderBlockIteratorNext() { // Order doesn't matter.
		info := a.blockInfoAt(blk.ID())
		lastUses, outs := info.lastUses, info.liveOuts
		for use, pc := range lastUses {
			// Usage without live-outs is a kill.
			if _, ok := outs[use]; !ok {
				info.kills[use] = pc
			}
		}

		if wazevoapi.RegAllocLoggingEnabled {
			fmt.Printf("\nfinalized info for block[%d]:\n%s\n", blk.ID(), info)
		}
	}
}

// upAndMarkStack is the Algorithm 9.10. in the book named Up_and_Mark_Stack(B, v).
// The only difference is that we don't have phis; instead, we have multiple defs in predecessors for such a VReg.
//
// We recursively call this, so passing `depth` for debugging.
func (a *Allocator) upAndMarkStack(b Block, v VReg, depth int) {
	if wazevoapi.RegAllocLoggingEnabled {
		fmt.Printf("%supAndMarkStack for %v at %v\n", strings.Repeat("\t", depth), v, b.ID())
	}

	info := a.blockInfoAt(b.ID())
	if _, ok := info.defs[v]; ok {
		return // Defined in this block, so no need to go further climbing up.
	}
	// v must be in liveIns.
	if _, ok := info.liveIns[v]; ok {
		return // But this case, it is already visited. (maybe by, for example, sibling blocks).
	}
	if wazevoapi.RegAllocLoggingEnabled {
		fmt.Printf("%sadding %v live-in at block[%d]\n", strings.Repeat("\t", depth), v, b.ID())
	}

	// Now we can safely mark v as a part of live-in
	info.liveIns[v] = struct{}{}
	preds := b.Preds()
	if len(preds) == 0 {
		panic(fmt.Sprintf("BUG: block has no predecessors while requiring live-in: blk%d", b.ID()))
	}

	// and climb up the CFG.
	for _, pred := range preds {
		if wazevoapi.RegAllocLoggingEnabled {
			fmt.Printf("%sadding %v live-out at block[%d]\n", strings.Repeat("\t", depth+1), v, pred.ID())
		}
		a.blockInfoAt(pred.ID()).liveOuts[v] = struct{}{}
		a.upAndMarkStack(pred, v, depth+1)
	}
}

func (a *Allocator) buildLiveRanges(f Function) {
	for blk := f.PostOrderBlockIteratorBegin(); blk != nil; blk = f.PostOrderBlockIteratorNext() { // Order doesn't matter.
		blkID := blk.ID()
		info := a.blockInfoAt(blkID)
		a.buildLiveRangesForNonReals(blkID, info)
		a.buildLiveRangesForReals(blkID, info)
		// Sort the live range for a fast lookup to find live registers at a given program counter.
		sort.Slice(info.liveNodes, func(i, j int) bool {
			inode, jnode := &info.liveNodes[i], &info.liveNodes[j]
			irange, jrange := inode.n.ranges[inode.rangeIndex], jnode.n.ranges[jnode.rangeIndex]
			if irange.begin == jrange.begin {
				return irange.end < jrange.end
			}
			return irange.begin < jrange.begin
		})
	}
}

func (a *Allocator) buildLiveRangesForNonReals(blkID int, info *blockInfo) {
	ins, outs, defs, kills := info.liveIns, info.liveOuts, info.defs, info.kills

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
			begin, end = 0, math.MaxInt64
			if _, ok := kills[v]; ok {
				panic("BUG: v is live-out but also killed")
			}
		} else {
			killPos, ok := kills[v]
			if !ok {
				panic("BUG: v is live-in but not live-out or use")
			}
			// v is killed at killPos.
			begin, end = 0, killPos
		}
		n := a.getOrAllocateNode(v)
		rangeIndex := len(n.ranges)
		n.ranges = append(n.ranges, liveRange{blockID: blkID, begin: begin, end: end})
		info.liveNodes = append(info.liveNodes, liveNodeInBlock{rangeIndex, n})
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
		var end programCounter
		if _, ok := outs[v]; ok {
			// v is defined here and live-out, so it is live-through.
			end = math.MaxInt64
			if _, ok := kills[v]; ok {
				panic("BUG: v is killed here but also killed")
			}
		} else {
			killPos, ok := kills[v]
			if !ok {
				// This case the defined value is not used at all.
				end = defPos
			} else {
				// v is killed at pos.
				end = killPos
			}
		}
		n := a.getOrAllocateNode(v)
		rangeIndex := len(n.ranges)
		n.ranges = append(n.ranges, liveRange{blockID: blkID, begin: defPos, end: end})
		info.liveNodes = append(info.liveNodes, liveNodeInBlock{rangeIndex, n})
	}

	// Reuse for the next block.
	a.vs = vs[:0]

	if wazevoapi.RegAllocValidationEnabled {
		for u := range kills {
			if !u.IsRealReg() {
				_, defined := defs[u]
				_, liveIn := ins[u]
				if !defined && !liveIn {
					panic(fmt.Sprintf("BUG: %v is killed but not defined or live-in", u))
				}
			}
		}
	}
}

// buildLiveRangesForReals builds live ranges for pre-colored real registers.
func (a *Allocator) buildLiveRangesForReals(blkID int, info *blockInfo) {
	ds, us := info.realRegDefs, info.realRegUses

	// In order to do the deterministic compilation, we need to sort the registers.
	a.vs = a.vs[:0]
	for v := range us {
		// Non allocation target registers are not needed here.
		if _, ok := a.allocatableSet[v.RealReg()]; !ok {
			continue
		}
		a.vs = append(a.vs, v)
	}
	sort.SliceStable(a.vs, func(i, j int) bool {
		return a.vs[i].RealReg() < a.vs[j].RealReg()
	})

	for _, v := range a.vs {
		uses := us[v]
		defs, ok := ds[v]
		if !ok || len(defs) != len(uses) {
			// This is likely a bug of the Instr interface implementation and/or ABI around call instructions.
			// E.g. call or ret instructions should specify that they use all the real registers (calling convention).
			panic(
				fmt.Sprintf(
					"BUG: real register (%s) is defined and used, but the number of defs and uses are different: %d (defs) != %d (uses)",
					a.regInfo.RealRegName(v.RealReg()), len(defs), len(uses),
				),
			)
		}

		for i := range uses {
			n := a.allocateNode()
			n.r = v.RealReg()
			n.v = v
			defined, used := defs[i], uses[i]
			n.ranges = append(n.ranges, liveRange{blockID: blkID, begin: defined, end: used})
			info.liveNodes = append(info.liveNodes, liveNodeInBlock{0, n})
		}
	}
}

// Reset resets the allocator's internal state so that it can be reused.
func (a *Allocator) Reset() {
	a.nodePool.Reset()
	a.blockInfos = a.blockInfos[:0]
	for i := range a.vRegIDToNode {
		a.vRegIDToNode[i] = nil
	}
	rr := a.realRegs[:0]
	for r := range a.allocatableSet {
		rr = append(rr, r)
	}
	for _, r := range rr {
		delete(a.allocatableSet, r)
	}
	rr = rr[:0]
	for r := range a.allocatedRegSet {
		rr = append(rr, r)
	}
	for _, r := range rr {
		delete(a.allocatedRegSet, r)
	}

	a.vs = a.vs[:0]
	a.nodes1 = a.nodes1[:0]
	for n := range a.nodeSet {
		a.nodes1 = append(a.nodes1, n)
	}
	for _, n := range a.nodes1 {
		delete(a.nodeSet, n)
	}
	a.nodes1 = a.nodes1[:0]
	a.nodes2 = a.nodes2[:0]
	a.realRegs = rr[:0]
}

func (a *Allocator) allocateBlockInfo(blockID int) {
	if blockID >= len(a.blockInfos) {
		a.blockInfos = append(a.blockInfos, make([]blockInfo, blockID+1)...)
	}
	info := &a.blockInfos[blockID]
	a.initBlockInfo(info)
}

func (a *Allocator) blockInfoAt(blockID int) (info *blockInfo) {
	info = &a.blockInfos[blockID]
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

func (a *Allocator) allocateNode() (n *node) {
	n = a.nodePool.Allocate()
	n.ranges = n.ranges[:0]
	n.copyFromVReg = nil
	n.copyToVReg = nil
	n.copyFromReal = RealRegInvalid
	n.copyToReal = RealRegInvalid
	// TODO: reuse!!
	n.neighbors = make(map[*node]struct{})
	return
}

func resetMap[T any](a *Allocator, m map[VReg]T) {
	a.vs = a.vs[:0]
	for v := range m {
		a.vs = append(a.vs, v)
	}
	for _, v := range a.vs {
		delete(m, v)
	}
}

func (a *Allocator) initBlockInfo(i *blockInfo) {
	i.liveNodes = i.liveNodes[:0]
	if i.liveOuts == nil {
		i.liveOuts = make(map[VReg]struct{})
	} else {
		resetMap(a, i.liveOuts)
	}
	if i.liveIns == nil {
		i.liveIns = make(map[VReg]struct{})
	} else {
		resetMap(a, i.liveIns)
	}
	if i.defs == nil {
		i.defs = make(map[VReg]programCounter)
	} else {
		resetMap(a, i.defs)
	}
	if i.lastUses == nil {
		i.lastUses = make(map[VReg]programCounter)
	} else {
		resetMap(a, i.lastUses)
	}
	if i.kills == nil {
		i.kills = make(map[VReg]programCounter)
	} else {
		resetMap(a, i.kills)
	}
	if i.realRegUses == nil {
		i.realRegUses = make(map[VReg][]programCounter)
	} else {
		resetMap(a, i.realRegUses)
	}
	if i.realRegDefs == nil {
		i.realRegDefs = make(map[VReg][]programCounter)
	} else {
		resetMap(a, i.realRegDefs)
	}
}

func (i *blockInfo) addRealRegUsage(v VReg, pc programCounter) {
	defs := i.realRegDefs[v]
	if len(defs) == 0 {
		// If the definition not found yet but used, this must be a function preamble,
		// so we let's assume it is defined at the beginning.
		i.realRegDefs[v] = append(i.realRegDefs[v], 0)
	}
	i.realRegUses[v] = append(i.realRegUses[v], pc)
}

// String implements fmt.Stringer for debugging.
func (i *blockInfo) String() string {
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
	for v, pos := range i.lastUses {
		buf.WriteString(fmt.Sprintf("%v@%v ", v, pos))
	}
	buf.WriteString("\n\tkills: ")
	for v, pos := range i.kills {
		buf.WriteString(fmt.Sprintf("%v@%v ", v, pos))
	}
	buf.WriteString("\n\trealRegUses: ")
	for v, pos := range i.realRegUses {
		buf.WriteString(fmt.Sprintf("%v@%v ", v, pos))
	}
	buf.WriteString("\n\trealRegDefs: ")
	for v, pos := range i.realRegDefs {
		buf.WriteString(fmt.Sprintf("%v@%v ", v, pos))
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
	buf.WriteString(" ranges[")
	for _, r := range n.ranges {
		buf.WriteString(fmt.Sprintf("[%v-%v]@blk%d ", r.begin, r.end, r.blockID))
	}
	buf.WriteString("]")
	// Add neighbors
	buf.WriteString(" neighbors[")
	for n := range n.neighbors {
		buf.WriteString(fmt.Sprintf("v%v ", n.v.ID()))
	}
	buf.WriteString("]")
	return buf.String()
}

func (n *node) spill() bool {
	return n.r == RealRegInvalid
}

// intersects returns true if the two live ranges intersect.
// Note that this doesn't compare the block ID because this is called to compare two intervals in the same block.
func (l *liveRange) intersects(other *liveRange) bool {
	return other.begin <= l.end && l.begin <= other.end
}

func (r *RegisterInfo) isCalleeSaved(reg RealReg) bool {
	_, ok := r.CalleeSavedRegisters[reg]
	return ok
}

func (r *RegisterInfo) isCallerSaved(reg RealReg) bool {
	_, ok := r.CallerSavedRegisters[reg]
	return ok
}

// String implements fmt.Stringer for debugging.
func (l *liveNodeInBlock) String() string {
	r := l.n.ranges[l.rangeIndex]
	return fmt.Sprintf("v%d@[%v-%v]", l.n.v.ID(), r.begin, r.end)
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
