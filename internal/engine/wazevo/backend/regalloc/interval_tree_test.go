package regalloc

import (
	"fmt"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"sort"
	"testing"
)

func TestIntervalTree_reset(t *testing.T) {
	tree := newIntervalTree()
	n := tree.allocator.Allocate()
	tree.root = n
	tree.reset()

	require.Nil(t, tree.root)
	require.Equal(t, 0, tree.allocator.Allocated())
}

func TestIntervalTreeInsert(t *testing.T) {
	n1 := &node{}
	tree := newIntervalTree()
	tree.insert(n1, 100, 200)
	require.NotNil(t, tree.root)
	require.NotNil(t, tree.root.nodes)
	require.Equal(t, n1, tree.root.nodes[0])
	n, ok := tree.intervals[intervalTreeNodeKey(100, 200)]
	require.True(t, ok)
	require.Equal(t, n1, n.nodes[0])
}

func TestIntervalTreeNodeInsert(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		tree := newIntervalTree()
		var n *intervalTreeNode
		allocated := n.insert(tree, &node{}, 0, 100)
		require.Equal(t, 1, tree.allocator.Allocated())
		require.NotNil(t, allocated)
		require.Equal(t, allocated, tree.allocator.View(0))
		require.Equal(t, programCounter(100), allocated.maxEnd)
		require.Equal(t, programCounter(0), allocated.begin)
		require.Equal(t, programCounter(100), allocated.end)
		require.Equal(t, 1, len(allocated.nodes))
		n, ok := tree.intervals[intervalTreeNodeKey(0, 100)]
		require.True(t, ok)
		require.Equal(t, allocated, n)
	})
	t.Run("left", func(t *testing.T) {
		tree := newIntervalTree()
		n := &intervalTreeNode{begin: 50, end: 100, maxEnd: 100}
		n1 := &node{}
		self := n.insert(tree, n1, 0, 200)
		require.Equal(t, self, n)
		require.Equal(t, 1, tree.allocator.Allocated())
		left := tree.allocator.View(0)
		require.Equal(t, n.left, left)
		require.Nil(t, n.right)
		require.Equal(t, programCounter(200), n.maxEnd)
		require.Equal(t, left.nodes[0], n1)
	})
	t.Run("right", func(t *testing.T) {
		tree := newIntervalTree()
		n := &intervalTreeNode{begin: 50, end: 100, maxEnd: 100}
		n1 := &node{}
		self := n.insert(tree, n1, 150, 200)
		require.Equal(t, self, n)
		require.Equal(t, 1, tree.allocator.Allocated())
		right := tree.allocator.View(0)
		require.Equal(t, n.right, right)
		require.Nil(t, n.left)
		require.Equal(t, programCounter(200), n.maxEnd)
		require.Equal(t, right.nodes[0], n1)
	})
}

type (
	interval struct {
		begin, end programCounter
		id         int
	}
	queryCase struct {
		query programCounter
		exp   []int
	}
)

func newQueryCase(s programCounter, exp ...int) queryCase {
	return queryCase{query: s, exp: exp}
}

func TestIntervalTree_collectActiveNodesAt(t *testing.T) {

	for _, tc := range []struct {
		name       string
		intervals  []interval
		queryCases []queryCase
	}{
		{
			name:      "single",
			intervals: []interval{{begin: 0, end: 100, id: 0}},
			queryCases: []queryCase{
				newQueryCase(0, 0),
				newQueryCase(0, 0),
				newQueryCase(1, 0),
				newQueryCase(1, 0),
				newQueryCase(101),
			},
		},
		{
			name:      "single/2",
			intervals: []interval{{begin: 50, end: 100, id: 0}},
			queryCases: []queryCase{
				newQueryCase(50, 0),
				newQueryCase(50, 0),
				newQueryCase(51, 0),
				newQueryCase(51, 0),
				newQueryCase(101),
				newQueryCase(48),
			},
		},
		{
			name:      "same id for different intervals",
			intervals: []interval{{begin: 50, end: 100, id: 0xa}, {begin: 150, end: 200, id: 0xa}},
			queryCases: []queryCase{
				newQueryCase(0),
				newQueryCase(50, 0xa),
				newQueryCase(101),
				newQueryCase(150, 0xa),
				newQueryCase(101),
			},
		},
		{
			name:      "two disjoint intervals",
			intervals: []interval{{begin: 50, end: 100, id: 0xa}, {begin: 150, end: 200, id: 0xb}},
			queryCases: []queryCase{
				newQueryCase(50, 0xa),
				newQueryCase(0),
				newQueryCase(51, 0xa),
				newQueryCase(101),
				newQueryCase(150, 0xb),
				newQueryCase(200, 0xb),
				newQueryCase(101),
				newQueryCase(201),
			},
		},
		{
			name:      "two intersecting intervals",
			intervals: []interval{{begin: 50, end: 100, id: 0xa}, {begin: 51, end: 200, id: 0xb}},
			queryCases: []queryCase{
				newQueryCase(0),
				newQueryCase(70, 0xa, 0xb),
				newQueryCase(1),
				newQueryCase(50, 0xa),
				newQueryCase(51, 0xa, 0xb),
				newQueryCase(100, 0xa, 0xb),
				newQueryCase(101, 0xb),
				newQueryCase(49),
				newQueryCase(1001),
			},
		},
		{
			name:      "two enclosing interval",
			intervals: []interval{{begin: 50, end: 100, id: 0xa}, {begin: 25, end: 200, id: 0xb}, {begin: 40, end: 1000, id: 0xc}},
			queryCases: []queryCase{
				newQueryCase(24),
				newQueryCase(25, 0xb),
				newQueryCase(39, 0xb),
				newQueryCase(40, 0xb, 0xc),
				newQueryCase(100, 0xa, 0xb, 0xc),
				newQueryCase(99, 0xa, 0xb, 0xc),
				newQueryCase(100, 0xa, 0xb, 0xc),
				newQueryCase(50, 0xa, 0xb, 0xc),
				newQueryCase(51, 0xa, 0xb, 0xc),
				newQueryCase(101, 0xb, 0xc),
				newQueryCase(49, 0xb, 0xc),
				newQueryCase(200, 0xb, 0xc),
				newQueryCase(201, 0xc),
				newQueryCase(1000, 0xc),
				newQueryCase(1001),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree := newIntervalTree()
			var maxID int
			for _, inter := range tc.intervals {
				n := &node{id: inter.id, r: RealReg(1)}
				tree.insert(n, inter.begin, inter.end)
				if maxID < inter.id {
					maxID = inter.id
				}
				key := intervalTreeNodeKey(inter.begin, inter.end)
				inserted := tree.intervals[key]
				inserted.nodes = append(inserted.nodes, &node{v: VRegInvalid.SetRealReg(RealRegInvalid)}) // non-real reg should be ignored.
			}
			for _, qc := range tc.queryCases {
				t.Run(fmt.Sprintf("%d", qc.query), func(t *testing.T) {
					var collected []*node
					tree.collectActiveRealRegNodesAt(qc.query, &collected)
					require.Equal(t, len(qc.exp), len(collected))
					var foundIDs []int
					for _, n := range collected {
						foundIDs = append(foundIDs, n.id)
					}
					sort.Slice(foundIDs, func(i, j int) bool {
						return foundIDs[i] < foundIDs[j]
					})
					require.Equal(t, qc.exp, foundIDs)
				})
			}
		})
	}
}

func TestIntervalTree_collectActiveNonRealVRegsAt(t *testing.T) {

	for _, tc := range []struct {
		name       string
		intervals  []interval
		queryCases []queryCase
	}{
		{
			name:      "single",
			intervals: []interval{{begin: 0, end: 100, id: 0}},
			queryCases: []queryCase{
				newQueryCase(0, 0),
				newQueryCase(0, 0),
				newQueryCase(1, 0),
				newQueryCase(1, 0),
				newQueryCase(101),
			},
		},
		{
			name:      "single/2",
			intervals: []interval{{begin: 50, end: 100, id: 0}},
			queryCases: []queryCase{
				newQueryCase(50, 0),
				newQueryCase(50, 0),
				newQueryCase(51, 0),
				newQueryCase(51, 0),
				newQueryCase(101),
				newQueryCase(48),
			},
		},
		{
			name:      "same id for different intervals",
			intervals: []interval{{begin: 50, end: 100, id: 0xa}, {begin: 150, end: 200, id: 0xa}},
			queryCases: []queryCase{
				newQueryCase(0),
				newQueryCase(50, 0xa),
				newQueryCase(101),
				newQueryCase(150, 0xa),
				newQueryCase(101),
			},
		},
		{
			name:      "two disjoint intervals",
			intervals: []interval{{begin: 50, end: 100, id: 0xa}, {begin: 150, end: 200, id: 0xb}},
			queryCases: []queryCase{
				newQueryCase(50, 0xa),
				newQueryCase(0),
				newQueryCase(51, 0xa),
				newQueryCase(101),
				newQueryCase(150, 0xb),
				newQueryCase(200, 0xb),
				newQueryCase(101),
				newQueryCase(201),
			},
		},
		{
			name:      "two intersecting intervals",
			intervals: []interval{{begin: 50, end: 100, id: 0xa}, {begin: 51, end: 200, id: 0xb}},
			queryCases: []queryCase{
				newQueryCase(0),
				newQueryCase(70, 0xa, 0xb),
				newQueryCase(1),
				newQueryCase(50, 0xa),
				newQueryCase(51, 0xa, 0xb),
				newQueryCase(100, 0xa, 0xb),
				newQueryCase(101, 0xb),
				newQueryCase(49),
				newQueryCase(1001),
			},
		},
		{
			name:      "two enclosing interval",
			intervals: []interval{{begin: 50, end: 100, id: 0xa}, {begin: 25, end: 200, id: 0xb}, {begin: 40, end: 1000, id: 0xc}},
			queryCases: []queryCase{
				newQueryCase(24),
				newQueryCase(25, 0xb),
				newQueryCase(39, 0xb),
				newQueryCase(40, 0xb, 0xc),
				newQueryCase(100, 0xa, 0xb, 0xc),
				newQueryCase(99, 0xa, 0xb, 0xc),
				newQueryCase(100, 0xa, 0xb, 0xc),
				newQueryCase(50, 0xa, 0xb, 0xc),
				newQueryCase(51, 0xa, 0xb, 0xc),
				newQueryCase(101, 0xb, 0xc),
				newQueryCase(49, 0xb, 0xc),
				newQueryCase(200, 0xb, 0xc),
				newQueryCase(201, 0xc),
				newQueryCase(1000, 0xc),
				newQueryCase(1001),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree := newIntervalTree()
			var maxID int
			for _, inter := range tc.intervals {
				n := &node{id: inter.id, r: RealReg(1)}
				tree.insert(n, inter.begin, inter.end)
				if maxID < inter.id {
					maxID = inter.id
				}
				key := intervalTreeNodeKey(inter.begin, inter.end)
				inserted := tree.intervals[key]
				// They are ignored.
				inserted.nodes = append(inserted.nodes, &node{v: FromRealReg(1, RegTypeInt)})
				inserted.nodes = append(inserted.nodes, &node{v: FromRealReg(1, RegTypeFloat)})
				inserted.nodes = append(inserted.nodes, &node{v: VReg(1)})
			}
			for _, qc := range tc.queryCases {
				t.Run(fmt.Sprintf("%d", qc.query), func(t *testing.T) {
					var collected []*node
					tree.collectActiveNonRealVRegsAt(qc.query, &collected)
					require.Equal(t, len(qc.exp), len(collected))
					var foundIDs []int
					for _, n := range collected {
						foundIDs = append(foundIDs, n.id)
					}
					sort.Slice(foundIDs, func(i, j int) bool {
						return foundIDs[i] < foundIDs[j]
					})
					require.Equal(t, qc.exp, foundIDs)
				})
			}
		})
	}
}

func TestIntervalTreeNode_intersects(t *testing.T) {
	for _, tc := range []struct {
		rhs, lhs intervalTreeNode
		exp      bool
	}{
		{
			rhs: intervalTreeNode{begin: 0, end: 100},
			lhs: intervalTreeNode{begin: 0, end: 100},
			exp: true,
		},
		{
			rhs: intervalTreeNode{begin: 0, end: 100},
			lhs: intervalTreeNode{begin: 0, end: 99},
			exp: true,
		},
		{
			rhs: intervalTreeNode{begin: 0, end: 100},
			lhs: intervalTreeNode{begin: 1, end: 100},
			exp: true,
		},
		{
			rhs: intervalTreeNode{begin: 50, end: 100},
			lhs: intervalTreeNode{begin: 1, end: 49},
			exp: false,
		},
		{
			rhs: intervalTreeNode{begin: 50, end: 100},
			lhs: intervalTreeNode{begin: 1, end: 50},
			exp: true,
		},
		{
			rhs: intervalTreeNode{begin: 50, end: 100},
			lhs: intervalTreeNode{begin: 1, end: 51},
			exp: true,
		},
		{
			rhs: intervalTreeNode{begin: 50, end: 100},
			lhs: intervalTreeNode{begin: 99, end: 102},
			exp: true,
		},
	} {
		actual := tc.rhs.intersects(&tc.lhs)
		require.Equal(t, tc.exp, actual)
	}
}
