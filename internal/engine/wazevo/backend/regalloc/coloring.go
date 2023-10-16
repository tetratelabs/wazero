package regalloc

import (
	"fmt"
	"sort"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
)

// buildNeighbors builds the neighbors for each node in the interference graph.
// TODO: node coalescing by leveraging the info given by Instr.IsCopy().
func (a *Allocator) buildNeighbors(f Function) {
	for blk := f.PostOrderBlockIteratorBegin(); blk != nil; blk = f.PostOrderBlockIteratorNext() {
		lives := a.blockInfos[blk.ID()].liveNodes
		a.buildNeighborsByLiveNodes(lives)
	}
}

func (a *Allocator) buildNeighborsByLiveNodes(lives []liveNodeInBlock) {
	if len(lives) == 0 {
		// TODO: shouldn't this kind of block be removed before reg alloc?
		return
	}
	for i, src := range lives[:len(lives)-1] {
		srcRange := &src.n.ranges[src.rangeIndex]
		for _, dst := range lives[i+1:] {
			srcN, dstN := src.n, dst.n
			if dst == src || dstN == srcN {
				panic(fmt.Sprintf("BUG: %s and %s are the same node", src.n.v, dst.n.v))
			}
			dstRange := &dst.n.ranges[dst.rangeIndex]
			if dstRange.begin > srcRange.end {
				// liveNodes are sorted by the start program counter, so we can break here.
				break
			}

			if srcN.v.RegType() == dstN.v.RegType() && // Interfere only if they are the same type.
				srcRange.intersects(dstRange) {
				srcN.neighbors = append(srcN.neighbors, dst.n)
				dstN.neighbors = append(dstN.neighbors, src.n)
			}
		}
	}
}

// coloring does the graph coloring for both RegType(s).
// Since the graphs are disjoint per RegType, we do it by RegType separately.
func (a *Allocator) coloring() {
	a.collectNodesByRegType(RegTypeInt)
	a.coloringFor(a.regInfo.AllocatableRegisters[RegTypeInt])
	a.collectNodesByRegType(RegTypeFloat)
	a.coloringFor(a.regInfo.AllocatableRegisters[RegTypeFloat])
}

// collectNodesByRegType collects all the nodes that are of the given register type.
// The result is stored in Allocator.nodes1.
func (a *Allocator) collectNodesByRegType(regType RegType) {
	a.nodes1 = a.nodes1[:0]
	// Gather all the nodes that are of the given register type.
	for i := 0; i < a.nodePool.Allocated(); i++ {
		// TODO: when we implement the coalescing, we should skip the coalesced nodes here.
		n := a.nodePool.View(i)
		if n.v.RegType() == regType {
			a.nodes1 = append(a.nodes1, n)
		}
	}
}

// coloringFor colors the graph by the given allocatable registers. The algorithm here is called "Chaitin's Algorithm".
//
// This assumes that the coloring target nodes are stored at Allocator.nodes1.
//
// TODO: the implementation here is not optimized at all. Come back later.
func (a *Allocator) coloringFor(allocatable []RealReg) {
	degreeSortedNodes := a.nodes1 // We assume nodes1 holds all the nodes of the given register type.
	// Reuses the nodes2 slice and the degrees map from the previous iteration.
	coloringStack := a.nodes2[:0]

	numAllocatable := len(allocatable)

	// Initialize the degree for each node which is defined as the number of neighbors.
	for _, n := range degreeSortedNodes {
		n.degree = len(n.neighbors)
	}

	// Sort the nodes by the current degree.
	sort.SliceStable(degreeSortedNodes, func(i, j int) bool {
		return degreeSortedNodes[i].degree < degreeSortedNodes[j].degree
	})

	// First step of the algorithm:
	// until we have removed the all the nodes:
	//	1. pop the nodes with degree < numAllocatable.
	//  2. if there's no node with degree < numAllocatable, spill one node.
	popTargetQueue := a.nodes3[:0] // Only containing the nodes whose degree < numAllocatable.
	for i := 0; i < len(degreeSortedNodes); i++ {
		n := degreeSortedNodes[i]
		if n.degree < numAllocatable {
			popTargetQueue = append(popTargetQueue, n)
			n.visited = true
		} else {
			break
		}
	}
	total := len(degreeSortedNodes)
	for len(coloringStack) != total {
		if len(popTargetQueue) == 0 {
			// If no node can be popped, it means that the graph is not colorable. We need to forcibly choose one node to pop.
			// TODO: currently we just choose the last node. We could do this more wisely. e.g. choose the one without pre-colored neighbors etc.
			// Swap the top node with the last node.
			tail := len(degreeSortedNodes) - 1
			for i := 0; i < len(degreeSortedNodes); i++ {
				j := tail - i
				n := degreeSortedNodes[j]
				if !n.visited {
					popTargetQueue = append(popTargetQueue, n)
					n.visited = true
					break
				}
			}
		}

		for len(popTargetQueue) > 0 {
			top := popTargetQueue[0]
			popTargetQueue = popTargetQueue[1:]
			for _, neighbor := range top.neighbors {
				neighbor.degree--
				if neighbor.degree < numAllocatable {
					if !neighbor.visited {
						popTargetQueue = append(popTargetQueue, neighbor)
						neighbor.visited = true
					}
				}
			}
			coloringStack = append(coloringStack, top)
		}
	}

	// Assign colors.
	neighborColorsSet := &a.realRegSet
	tail := len(coloringStack) - 1
	for i := range coloringStack {
		n := coloringStack[tail-i]
		if n.r != RealRegInvalid {
			// This means the node is a pre-colored register.
			continue
		}

		if wazevoapi.RegAllocLoggingEnabled {
			fmt.Printf("coloring %s\n", n)
		}

		// Gather already used colors.
		for _, neighbor := range n.neighbors {
			if neighborColor := neighbor.r; neighborColor != RealRegInvalid {
				neighborColorsSet[neighborColor] = true
			}
		}

		a.assignColor(n, neighborColorsSet, allocatable)

		if wazevoapi.RegAllocLoggingEnabled {
			fmt.Printf("\tassigned color: %s\n", a.regInfo.RealRegName(n.r))
		}

		// Reset the map for the next iteration.
		for j := range neighborColorsSet {
			neighborColorsSet[j] = false
		}
	}

	if wazevoapi.RegAllocValidationEnabled {
		for _, n := range coloringStack {
			if n.r == RealRegInvalid {
				continue
			}
			for _, neighbor := range n.neighbors {
				if n.r == neighbor.r {
					panic(fmt.Sprintf("BUG color conflict: %s vs %s", n.v, neighbor.v))
				}
			}
		}
	}

	// Reuses the slices for the next coloring.
	a.nodes1 = degreeSortedNodes[:0]
	a.nodes2 = coloringStack[:0]
	a.nodes3 = popTargetQueue[:0]
}

func (a *Allocator) assignColor(n *node, neighborColorsSet *[128]bool, allocatable []RealReg) {
	if cfv := n.copyFromVReg; cfv != nil && cfv.r != RealRegInvalid {
		r := cfv.r
		if a.allocatableSet[r] {
			if !neighborColorsSet[r] {
				n.r = r
				a.allocatedRegSet[r] = true
				return
			}
		}
	}

	if ctv := n.copyToVReg; ctv != nil && ctv.r != RealRegInvalid {
		r := ctv.r
		if a.allocatableSet[r] {
			if !neighborColorsSet[r] {
				n.r = r
				a.allocatedRegSet[r] = true
				return
			}
		}
	}

	if r := n.copyFromReal; r != RealRegInvalid {
		if a.allocatableSet[r] {
			if !neighborColorsSet[r] {
				n.r = r
				a.allocatedRegSet[r] = true
				return
			}
		}
	}

	if r := n.copyToReal; r != RealRegInvalid {
		if a.allocatableSet[r] {
			if !neighborColorsSet[r] {
				n.r = r
				a.allocatedRegSet[r] = true
				return
			}
		}
	}

	if n.r == RealRegInvalid {
		for _, color := range allocatable {
			if !neighborColorsSet[color] {
				n.r = color
				a.allocatedRegSet[color] = true
				break
			}
		}
	}
}
