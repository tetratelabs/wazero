package regalloc

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

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
			expRegs: []RealReg{2, 1},
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
			expRegs:     []RealReg{2, 1, RealRegInvalid},
		},
		{
			//     0
			//   /   \
			//  1 --- 2 (precolor)
			name:         "three nodes with interference and spill / precolor",
			allocatable:  []RealReg{RealReg(1), RealReg(2)},
			links:        [][]int{{1, 2}, {2}, {}},
			expRegs:      []RealReg{RealRegInvalid, 1, 2},
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
			expRegs: []RealReg{40, 10, 20, 30, 30, 20, 10},
		},
		{
			// https://web.stanford.edu/class/archive/cs/cs143/cs143.1128/lectures/17/Slides17.pdf
			name:        "example in page 169",
			allocatable: []RealReg{10, 20, 30},
			links: [][]int{
				{1, 2, 3}, {2, 3, 4, 5}, {3, 4}, {}, {5}, {}, {},
			},
			expRegs: []RealReg{30, RealRegInvalid, 20, 10, 10, 20, 10},
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
					a.maybeAddEdge(n1, n2)
				}
			}
			a.finalizeEdges()
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
