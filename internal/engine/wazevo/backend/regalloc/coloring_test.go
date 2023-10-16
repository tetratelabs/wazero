package regalloc

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestAllocator_buildNeighborsByLiveNodes(t *testing.T) {
	for _, tc := range []struct {
		name          string
		lives         []liveNodeInBlock
		expectedEdges [][2]int
	}{
		{name: "empty", lives: []liveNodeInBlock{}},
		{
			name: "one node",
			lives: []liveNodeInBlock{
				{rangeIndex: 0, n: &node{ranges: []liveRange{{begin: 0, end: 1}}}},
			},
		},
		{
			name: "no overlap",
			lives: []liveNodeInBlock{
				{rangeIndex: 4, n: &node{ranges: []liveRange{
					{}, {}, {}, {}, {begin: 0, end: 1},
				}}},
				{rangeIndex: 1, n: &node{v: VReg(0).SetRegType(RegTypeInt), ranges: []liveRange{
					{}, {begin: 2, end: 3},
				}}},
				// This overlaps with the above, but is not the same type.
				{rangeIndex: 0, n: &node{v: VReg(1).SetRegType(RegTypeFloat), ranges: []liveRange{
					{begin: 2, end: 3},
				}}},
			},
		},
		{
			name: "overlap",
			lives: []liveNodeInBlock{
				{rangeIndex: 0, n: &node{v: VReg(0).SetRegType(RegTypeInt), ranges: []liveRange{
					{begin: 2, end: 50},
				}}},
				{rangeIndex: 0, n: &node{v: VReg(1).SetRegType(RegTypeInt), ranges: []liveRange{
					{begin: 2, end: 3},
				}}},
				// This overlaps with the above, but is not the same type.
				{rangeIndex: 0, n: &node{v: VReg(2).SetRegType(RegTypeFloat), ranges: []liveRange{
					{begin: 2, end: 100},
				}}},
				{rangeIndex: 0, n: &node{v: VReg(3).SetRegType(RegTypeFloat), ranges: []liveRange{
					{begin: 100, end: 100},
				}}},
			},
			expectedEdges: [][2]int{
				{0, 1}, {2, 3},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := NewAllocator(&RegisterInfo{})

			a.buildNeighborsByLiveNodes(tc.lives)

			expectedNeighborCounts := map[*node]int{}
			for _, edge := range tc.expectedEdges {
				i1, i2 := edge[0], edge[1]
				n1, n2 := tc.lives[i1].n, tc.lives[i2].n

				var found bool
				for _, neighbor := range n2.neighbors {
					if neighbor == n1 {
						found = true
						break
					}
				}
				require.True(t, found)
				found = false
				for _, neighbor := range n1.neighbors {
					if neighbor == n2 {
						found = true
						break
					}
				}
				require.True(t, found)
				expectedNeighborCounts[n1]++
				expectedNeighborCounts[n2]++
			}
			for _, n := range tc.lives {
				require.Equal(t, expectedNeighborCounts[n.n], len(n.n.neighbors))
			}
		})
	}
}

func TestAllocator_collectNodesByRegType(t *testing.T) {
	a := NewAllocator(&RegisterInfo{})
	n1 := a.allocateNode()
	n1.v = VReg(0).SetRegType(RegTypeInt)
	n2 := a.allocateNode()
	n2.v = VReg(1).SetRegType(RegTypeFloat)
	n3 := a.allocateNode()
	n3.v = VReg(2).SetRegType(RegTypeInt)
	n4 := a.allocateNode()
	n4.v = VReg(3).SetRegType(RegTypeInt)

	a.collectNodesByRegType(RegTypeInt)
	require.Equal(t, []*node{n1, n3, n4}, a.nodes1)
	a.collectNodesByRegType(RegTypeFloat)
	require.Equal(t, []*node{n2}, a.nodes1)
}

func TestAllocator_coloringFor(t *testing.T) {
	addEdge := func(n1, n2 *node) {
		n1.neighbors = append(n1.neighbors, n2)
		n2.neighbors = append(n2.neighbors, n1)
	}

	for _, tc := range []struct {
		name         string
		allocatable  []RealReg
		links        [][]int
		expRegs      []RealReg
		preColorRegs map[int]RealReg
	}{
		{
			name:        "one nodes",
			allocatable: []RealReg{1},
			links:       [][]int{{}},
			expRegs:     []RealReg{1},
		},
		{
			name:        "two nodes without interference",
			allocatable: []RealReg{1, 2},
			links:       [][]int{{}, {}},
			// No interference, so both can be assigned a register.
			expRegs: []RealReg{1, 1},
		},
		{
			name:        "two nodes with interference",
			allocatable: []RealReg{1, 2},
			links:       [][]int{{1}, {0}},
			// Interference, so only one can be assigned a register.
			expRegs: []RealReg{1, 2},
		},
		{
			// 0 <- 1 -> 2
			name:        "three nodes with interference but no spill",
			allocatable: []RealReg{1, 2},
			links:       [][]int{{}, {0, 2}, {}},
			expRegs:     []RealReg{2, 1, 2},
		},
		{
			// 0 <- 1 -> 2 (precolor)
			name:         "three nodes with interference but no spill / precolor",
			allocatable:  []RealReg{1, 2},
			links:        [][]int{{}, {0, 2}, {}},
			expRegs:      []RealReg{1, 2, 1},
			preColorRegs: map[int]RealReg{2: 1},
		},
		{
			//     0
			//   /   \
			//  1 --- 3
			name:        "three nodes with interference and spill",
			allocatable: []RealReg{RealReg(1), RealReg(2)},
			links:       [][]int{{1, 2}, {2}, {}},
			expRegs:     []RealReg{1, 2, RealRegInvalid},
		},
		{
			//     0
			//   /   \
			//  1 --- 2 (precolor)
			name:         "three nodes with interference and spill / precolor",
			allocatable:  []RealReg{RealReg(1), RealReg(2)},
			links:        [][]int{{1, 2}, {2}, {}},
			expRegs:      []RealReg{1, RealRegInvalid, 2},
			preColorRegs: map[int]RealReg{2: 2},
		},
		{
			// https://web.stanford.edu/class/archive/cs/cs143/cs143.1128/lectures/17/Slides17.pdf
			name:        "example in page 140",
			allocatable: []RealReg{10, 20, 30, 40},
			links: [][]int{
				{1, 3, 5, 6},
				{2, 3, 4},
				{3, 4},
				{5, 6},
				{5, 6},
				{6},
				{},
			},
			expRegs: []RealReg{40, 20, 30, 10, 10, 30, 20},
		},
		{
			// https://web.stanford.edu/class/archive/cs/cs143/cs143.1128/lectures/17/Slides17.pdf
			name:        "example in page 169",
			allocatable: []RealReg{10, 20, 30},
			links: [][]int{
				{1, 2, 3}, {2, 3, 4, 5}, {3, 4}, {}, {5}, {}, {},
			},
			expRegs: []RealReg{10, RealRegInvalid, 20, 30, 10, 20, 10},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.preColorRegs == nil {
				tc.preColorRegs = map[int]RealReg{}
			}
			a := NewAllocator(&RegisterInfo{})
			testNodes := make([]*node, 0, len(tc.expRegs))
			for i := range tc.expRegs {
				n := a.allocateNode()
				n.v = VReg(i)
				if r, ok := tc.preColorRegs[i]; ok {
					n.r = r
				}
				testNodes = append(testNodes, n)
				a.nodes1 = append(a.nodes1, n)
			}
			for i, links := range tc.links {
				n1 := testNodes[i]
				for _, link := range links {
					n2 := testNodes[link]
					addEdge(n1, n2)
				}
			}
			a.coloringFor(tc.allocatable)
			var actual []string
			for _, n := range testNodes {
				actual = append(actual, n.r.String())
			}
			var exp []string
			for _, r := range tc.expRegs {
				exp = append(exp, r.String())
			}
			require.Equal(t, exp, actual)
		})
	}
}

func TestAllocator_assignColor(t *testing.T) {
	t.Run("copyFromVReg", func(t *testing.T) {
		a := NewAllocator(&RegisterInfo{})
		a.allocatableSet[10] = true
		n := a.getOrAllocateNode(100)
		n.copyFromVReg = &node{r: 10}
		a.assignColor(n, &a.realRegSet, nil)
		require.Equal(t, RealReg(10), n.r)
		ok := a.allocatedRegSet[n.r]
		require.True(t, ok)
	})
	t.Run("copyToVReg", func(t *testing.T) {
		a := NewAllocator(&RegisterInfo{})
		a.allocatableSet[10] = true
		a.allocatableSet[20] = true
		n := a.getOrAllocateNode(100)
		n.copyFromVReg = &node{r: 10}
		n.copyToVReg = &node{r: 20}
		a.realRegSet[10] = true
		a.assignColor(n, &a.realRegSet, nil)
		require.Equal(t, RealReg(20), n.r)
		ok := a.allocatedRegSet[n.r]
		require.True(t, ok)
	})
	t.Run("copyFromReal", func(t *testing.T) {
		a := NewAllocator(&RegisterInfo{})
		a.allocatableSet[10] = true
		a.allocatableSet[20] = true
		a.allocatableSet[30] = true
		n := a.getOrAllocateNode(100)
		n.copyFromVReg = &node{r: 10}
		n.copyToVReg = &node{r: 20}
		n.copyFromReal = 30
		a.realRegSet[10] = true
		a.realRegSet[20] = true
		a.assignColor(n, &a.realRegSet, nil)
		require.Equal(t, RealReg(30), n.r)
		ok := a.allocatedRegSet[n.r]
		require.True(t, ok)
	})
	t.Run("copyToReal", func(t *testing.T) {
		a := NewAllocator(&RegisterInfo{})
		a.allocatableSet[10] = true
		a.allocatableSet[20] = true
		a.allocatableSet[30] = true
		a.allocatableSet[40] = true
		n := a.getOrAllocateNode(100)
		n.copyFromVReg = &node{r: 10}
		n.copyToVReg = &node{r: 20}
		n.copyFromReal = 30
		n.copyToReal = 40
		a.realRegSet[10] = true
		a.realRegSet[20] = true
		a.realRegSet[30] = true
		a.assignColor(n, &a.realRegSet, nil)
		require.Equal(t, RealReg(40), n.r)
		ok := a.allocatedRegSet[n.r]
		require.True(t, ok)
	})
	t.Run("from allocatable sets", func(t *testing.T) {
		a := NewAllocator(&RegisterInfo{})
		a.allocatableSet[10] = true
		a.allocatableSet[20] = true
		a.allocatableSet[30] = true
		a.allocatableSet[40] = true
		a.allocatableSet[50] = true
		n := a.getOrAllocateNode(100)
		n.copyFromVReg = &node{r: 10}
		n.copyToVReg = &node{r: 20}
		n.copyFromReal = 30
		n.copyToReal = 40
		a.realRegSet[10] = true
		a.realRegSet[20] = true
		a.realRegSet[30] = true
		a.realRegSet[40] = true
		a.assignColor(n, &a.realRegSet, []RealReg{50})
		require.Equal(t, RealReg(50), n.r)
		ok := a.allocatedRegSet[n.r]
		require.True(t, ok)
	})
}
