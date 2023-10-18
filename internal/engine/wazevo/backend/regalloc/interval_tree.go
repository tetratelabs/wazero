package regalloc

import "github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"

type intervalTree struct {
	root      *intervalTreeNode
	allocator intervalTreeNodeAllocator
	intervals map[uint64]*intervalTreeNode
}

func intervalTreeNodeKey(begin, end programCounter) uint64 {
	return uint64(begin) | uint64(end)<<32
}

func (t *intervalTree) insert(n *node, begin, end programCounter) *intervalTreeNode {
	key := uint64(begin) | uint64(end)<<32
	if i, ok := t.intervals[key]; ok {
		i.nodes = append(i.nodes, n)
		return i
	}
	t.root = t.root.insert(t, n, begin, end)
	ret := t.intervals[key]
	t.buildNeighbors(ret) // TODO: this can be done while inserting.
	return ret
}

func (t *intervalTree) reset() {
	t.root = nil
	t.allocator.Reset()
	t.intervals = make(map[uint64]*intervalTreeNode)
}

func newIntervalTree() *intervalTree {
	return &intervalTree{
		allocator: wazevoapi.NewPool[intervalTreeNode](),
		intervals: make(map[uint64]*intervalTreeNode),
	}
}

type intervalTreeNodeAllocator = wazevoapi.Pool[intervalTreeNode]

type intervalTreeNode struct {
	begin, end  programCounter
	nodes       []*node
	maxEnd      programCounter
	neighbors   []*intervalTreeNode
	left, right *intervalTreeNode
	// TODO: color for red-black balancing.
}

func (i *intervalTreeNode) insert(t *intervalTree, n *node, begin, end programCounter) *intervalTreeNode {
	if i == nil {
		intervalNode := t.allocator.Allocate()
		intervalNode.right = nil
		intervalNode.left = nil
		intervalNode.nodes = append(intervalNode.nodes, n)
		intervalNode.maxEnd = end
		intervalNode.begin = begin
		intervalNode.end = end
		key := uint64(begin) | uint64(end)<<32
		t.intervals[key] = intervalNode
		return intervalNode
	}
	if begin < i.begin {
		i.left = i.left.insert(t, n, begin, end)
	} else {
		i.right = i.right.insert(t, n, begin, end)
	}
	if i.maxEnd < end {
		i.maxEnd = end
	}

	// TODO: balancing logic so that collection functions are faster.

	return i
}

func (t *intervalTree) buildNeighbors(from *intervalTreeNode) {
	t.root.buildNeighbors(from)
}

func (i *intervalTreeNode) buildNeighbors(from *intervalTreeNode) {
	if i == nil {
		return
	}
	if i.intersects(from) {
		from.neighbors = append(from.neighbors, i)
		i.neighbors = append(i.neighbors, from)
	}
	if i.left != nil && i.left.maxEnd >= from.begin {
		i.left.buildNeighbors(from)
	}
	if i.begin <= from.end {
		i.right.buildNeighbors(from)
	}
}

func (t *intervalTree) collectActiveNonRealVRegsAt(pc programCounter, overlaps *[]*node) {
	*overlaps = (*overlaps)[:0]
	t.root.collectActiveNonRealVRegsAt(pc, overlaps)
}

func (i *intervalTreeNode) collectActiveNonRealVRegsAt(pc programCounter, overlaps *[]*node) {
	if i == nil {
		return
	}
	if i.begin <= pc && i.end >= pc {
		for _, n := range i.nodes {
			if n.spill() || n.v.IsRealReg() {
				continue
			}
			*overlaps = append(*overlaps, n)
		}
	}
	if i.left != nil && i.left.maxEnd >= pc {
		i.left.collectActiveNonRealVRegsAt(pc, overlaps)
	}
	if i.begin <= pc {
		i.right.collectActiveNonRealVRegsAt(pc, overlaps)
	}
}

func (t *intervalTree) collectActiveRealRegNodesAt(pc programCounter, overlaps *[]*node) {
	*overlaps = (*overlaps)[:0]
	t.root.collectActiveRealRegNodesAt(pc, overlaps)
}

func (i *intervalTreeNode) collectActiveRealRegNodesAt(pc programCounter, overlaps *[]*node) {
	if i == nil {
		return
	}
	if i.begin <= pc && i.end >= pc {
		for _, n := range i.nodes {
			if n.assignedRealReg() != RealRegInvalid {
				*overlaps = append(*overlaps, n)
			}
		}
	}
	if i.left != nil && i.left.maxEnd >= pc {
		i.left.collectActiveRealRegNodesAt(pc, overlaps)
	}
	if i.begin <= pc {
		i.right.collectActiveRealRegNodesAt(pc, overlaps)
	}
}

func (i *intervalTreeNode) intersects(j *intervalTreeNode) bool {
	return i.begin <= j.end && i.end >= j.begin || j.begin <= i.end && j.end >= i.begin
}
