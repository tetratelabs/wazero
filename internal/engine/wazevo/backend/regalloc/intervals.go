package regalloc

import (
	"sort"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
)

type intervalManager struct {
	allocator       wazevoapi.Pool[interval]
	intervals       map[uint64]*interval
	sortedIntervals []*interval
	collectionCur   int
}

func intervalTreeNodeKey(begin, end programCounter) uint64 {
	return uint64(begin) | uint64(end)<<32
}

func (t *intervalManager) insert(n *node, begin, end programCounter) *interval {
	key := intervalTreeNodeKey(begin, end)
	if i, ok := t.intervals[key]; ok {
		i.nodes = append(i.nodes, n)
		return i
	}
	i := t.allocator.Allocate()
	i.nodes = append(i.nodes, n)
	i.begin = begin
	i.end = end
	t.intervals[key] = i
	t.sortedIntervals = append(t.sortedIntervals, i) // Will be sorted later.
	return i
}

func (t *intervalManager) reset() {
	t.allocator.Reset()
	t.sortedIntervals = t.sortedIntervals[:0]
	t.intervals = make(map[uint64]*interval)
	t.collectionCur = 0
}

func newIntervalManager() *intervalManager {
	return &intervalManager{
		allocator: wazevoapi.NewPool[interval](resetIntervalTreeNode),
		intervals: make(map[uint64]*interval),
	}
}

type interval struct {
	begin, end programCounter
	nodes      []*node
	neighbors  []*interval
}

func resetIntervalTreeNode(i *interval) {
	i.begin = 0
	i.end = 0
	i.nodes = i.nodes[:0]
	i.neighbors = i.neighbors[:0]
}

func (t *intervalManager) build() {
	sort.Slice(t.sortedIntervals, func(i, j int) bool {
		ii, ij := t.sortedIntervals[i], t.sortedIntervals[j]
		if ii.begin == ij.begin {
			return ii.end < ij.end
		}
		return ii.begin < ij.begin
	})

	var cur int
	var existingEndMax programCounter = -1
	for i, _interval := range t.sortedIntervals {
		begin, end := _interval.begin, _interval.end
		if begin > existingEndMax {
			cur = i
			existingEndMax = end
		} else {
			for j := cur; j < i; j++ {
				existing := t.sortedIntervals[j]
				if existing.end < begin {
					continue
				}
				if existing.begin > end {
					panic("BUG")
				}
				_interval.neighbors = append(_interval.neighbors, existing)
				existing.neighbors = append(existing.neighbors, _interval)
			}
			if end > existingEndMax {
				existingEndMax = end
			}
		}
	}
}

func (t *intervalManager) collectActiveNodes(pc programCounter, collected *[]*node, onlyReal bool) {
	*collected = (*collected)[:0]

	// Advance the collection cursor until the current interval's end is greater than pc.
	l := len(t.sortedIntervals)
	for cur := t.collectionCur; cur < l; cur++ {
		curNode := t.sortedIntervals[cur]
		if curNode.end < pc {
			t.collectionCur++
			continue
		} else {
			break
		}
	}

	for cur := t.collectionCur; cur < l; cur++ {
		curNode := t.sortedIntervals[cur]
		if curNode.end < pc {
			continue
		} else if curNode.begin > pc {
			break
		}

		for _, n := range curNode.nodes {
			if onlyReal {
				if n.assignedRealReg() == RealRegInvalid {
					continue
				}
			} else {
				if n.spill() || n.v.IsRealReg() {
					continue
				}
			}
			*collected = append(*collected, n)
		}
	}
}
