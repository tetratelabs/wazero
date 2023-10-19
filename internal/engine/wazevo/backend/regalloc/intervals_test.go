package regalloc

import (
	"fmt"
	"sort"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestIntervalsManager_build(t *testing.T) {
	type (
		intervalCase struct {
			begin, end programCounter
		}
		expNeighborCase struct {
			index     int
			neighbors []intervalCase
		}
	)

	for _, tc := range []struct {
		name         string
		intervals    []intervalCase
		expNeighbors []expNeighborCase
	}{
		{
			name:         "single",
			intervals:    []intervalCase{{begin: 0, end: 100}},
			expNeighbors: []expNeighborCase{{index: 0}},
		},
		{
			name:         "disjoints",
			intervals:    []intervalCase{{begin: 50, end: 100}, {begin: 1, end: 2}},
			expNeighbors: []expNeighborCase{{index: 0}, {index: 1}},
		},
		{
			name:         "disjoints duplicate",
			intervals:    []intervalCase{{begin: 50, end: 100}, {begin: 1, end: 2}, {begin: 50, end: 100}, {begin: 1, end: 2}},
			expNeighbors: []expNeighborCase{{index: 0}, {index: 1}, {index: 2}, {index: 3}},
		},
		{
			name: "two intersecting",
			intervals: []intervalCase{
				{begin: 70, end: 200},
				{begin: 50, end: 100},
			},
			expNeighbors: []expNeighborCase{
				{index: 0, neighbors: []intervalCase{{begin: 50, end: 100}}},
				{index: 1, neighbors: []intervalCase{{begin: 70, end: 200}}},
			},
		},
		{
			name: "same beginnings",
			intervals: []intervalCase{
				{begin: 50, end: 200},
				{begin: 50, end: 401},
				{begin: 50, end: 201},
				{begin: 50, end: 302},
			},
			expNeighbors: []expNeighborCase{
				{index: 0, neighbors: []intervalCase{{begin: 50, end: 201}, {begin: 50, end: 302}, {begin: 50, end: 401}}},
				{index: 1, neighbors: []intervalCase{{begin: 50, end: 200}, {begin: 50, end: 201}, {begin: 50, end: 302}}},
				{index: 2, neighbors: []intervalCase{{begin: 50, end: 200}, {begin: 50, end: 302}, {begin: 50, end: 401}}},
				{index: 3, neighbors: []intervalCase{{begin: 50, end: 200}, {begin: 50, end: 201}, {begin: 50, end: 401}}},
			},
		},
		{
			name: "three intersecting",
			intervals: []intervalCase{
				{begin: 70, end: 200},
				{begin: 71, end: 150},
				{begin: 50, end: 100},
			},
			expNeighbors: []expNeighborCase{
				{index: 0, neighbors: []intervalCase{{begin: 50, end: 100}, {begin: 71, end: 150}}},
				{index: 1, neighbors: []intervalCase{{begin: 50, end: 100}, {begin: 70, end: 200}}},
				{index: 2, neighbors: []intervalCase{{begin: 70, end: 200}, {begin: 71, end: 150}}},
			},
		},
		{
			name: "two enclosing interval",
			intervals: []intervalCase{
				{begin: 50, end: 100},
				{begin: 25, end: 200},
				{begin: 40, end: 1000},
			},
			expNeighbors: []expNeighborCase{
				{index: 0, neighbors: []intervalCase{{begin: 25, end: 200}, {begin: 40, end: 1000}}},
				{index: 1, neighbors: []intervalCase{{begin: 40, end: 1000}, {begin: 50, end: 100}}},
				{index: 2, neighbors: []intervalCase{{begin: 25, end: 200}, {begin: 50, end: 100}}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := newIntervalManager()
			for i, inter := range tc.intervals {
				n := &node{id: i, r: RealReg(1)}
				manager.insert(n, inter.begin, inter.end)
			}
			manager.build()

			for i, exp := range tc.expNeighbors {
				it := tc.intervals[exp.index]
				key := intervalTreeNodeKey(it.begin, it.end)

				var found []intervalCase
				for _, n := range manager.intervals[key].neighbors {
					found = append(found, intervalCase{begin: n.begin, end: n.end})
				}
				sort.Slice(found, func(i, j int) bool {
					return found[i].begin < found[j].begin
				})
				require.Equal(t, exp.neighbors, found, fmt.Sprintf("case=%d", i))
			}
		})
	}
}

func TestIntervalManager_collectActiveNodes(t *testing.T) {
	type (
		queryCase struct {
			query programCounter
			exp   []int
		}
		intervalCase struct {
			begin, end programCounter
			id         int
		}
	)

	newQueryCase := func(s programCounter, exp ...int) queryCase {
		return queryCase{query: s, exp: exp}
	}

	for _, tc := range []struct {
		name       string
		intervals  []intervalCase
		queryCases []queryCase
	}{
		{
			name:      "single",
			intervals: []intervalCase{{begin: 0, end: 100, id: 0}},
			queryCases: []queryCase{
				newQueryCase(0, 0),
				newQueryCase(1, 0),
				newQueryCase(101),
			},
		},
		{
			name:      "single/2",
			intervals: []intervalCase{{begin: 50, end: 100, id: 0}},
			queryCases: []queryCase{
				newQueryCase(48),
				newQueryCase(50, 0),
				newQueryCase(51, 0),
				newQueryCase(101),
			},
		},
		{
			name:      "same id for different intervals",
			intervals: []intervalCase{{begin: 50, end: 100, id: 0xa}, {begin: 150, end: 200, id: 0xa}},
			queryCases: []queryCase{
				newQueryCase(0),
				newQueryCase(50, 0xa),
				newQueryCase(101),
				newQueryCase(150, 0xa),
			},
		},
		{
			name:      "two disjoint intervals",
			intervals: []intervalCase{{begin: 50, end: 100, id: 0xa}, {begin: 150, end: 200, id: 0xb}},
			queryCases: []queryCase{
				newQueryCase(0),
				newQueryCase(50, 0xa),
				newQueryCase(51, 0xa),
				newQueryCase(101),
				newQueryCase(150, 0xb),
				newQueryCase(200, 0xb),
				newQueryCase(201),
			},
		},
		{
			name:      "two intersecting intervals",
			intervals: []intervalCase{{begin: 50, end: 100, id: 0xa}, {begin: 51, end: 200, id: 0xb}},
			queryCases: []queryCase{
				newQueryCase(0),
				newQueryCase(1),
				newQueryCase(49),
				newQueryCase(50, 0xa),
				newQueryCase(51, 0xa, 0xb),
				newQueryCase(70, 0xa, 0xb),
				newQueryCase(100, 0xa, 0xb),
				newQueryCase(101, 0xb),
				newQueryCase(1001),
			},
		},
		{
			name:      "two enclosing interval",
			intervals: []intervalCase{{begin: 50, end: 100, id: 0xa}, {begin: 25, end: 200, id: 0xb}, {begin: 40, end: 1000, id: 0xc}},
			queryCases: []queryCase{
				newQueryCase(24),
				newQueryCase(25, 0xb),
				newQueryCase(39, 0xb),
				newQueryCase(40, 0xb, 0xc),
				newQueryCase(49, 0xb, 0xc),
				newQueryCase(50, 0xa, 0xb, 0xc),
				newQueryCase(51, 0xa, 0xb, 0xc),
				newQueryCase(99, 0xa, 0xb, 0xc),
				newQueryCase(100, 0xa, 0xb, 0xc),
				newQueryCase(101, 0xb, 0xc),
				newQueryCase(200, 0xb, 0xc),
				newQueryCase(201, 0xc),
				newQueryCase(1000, 0xc),
				newQueryCase(1001),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, onlyReal := range []bool{false, true} {
				t.Run(fmt.Sprintf("onlyReal=%t", onlyReal), func(t *testing.T) {
					manager := newIntervalManager()
					for _, inter := range tc.intervals {
						n := &node{id: inter.id, r: RealReg(1)}
						manager.insert(n, inter.begin, inter.end)
						key := intervalTreeNodeKey(inter.begin, inter.end)
						inserted := manager.intervals[key]

						// They are ignored.
						if onlyReal {
							inserted.nodes = append(inserted.nodes, &node{v: VRegInvalid.SetRealReg(RealRegInvalid)}) // non-real reg should be ignored.
						} else {
							inserted.nodes = append(inserted.nodes, &node{v: FromRealReg(1, RegTypeInt)})
							inserted.nodes = append(inserted.nodes, &node{v: FromRealReg(1, RegTypeFloat)})
							inserted.nodes = append(inserted.nodes, &node{v: VReg(1)})
						}
					}
					manager.build()
					for _, qc := range tc.queryCases {
						t.Run(fmt.Sprintf("%d", qc.query), func(t *testing.T) {
							var collected []*node
							manager.collectActiveNodes(qc.query, &collected, onlyReal)
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
		})
	}
}
