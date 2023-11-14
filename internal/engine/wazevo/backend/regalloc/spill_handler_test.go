package regalloc

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestSpillHandler_init(t *testing.T) {
	instr := newMockInstr().use(FromRealReg(30, RegTypeInt))
	var s spillHandler
	activeNodes := []*node{{r: RealReg(0)}, {r: RealReg(1)}, {r: RealReg(2)}, {v: VReg(10).SetRealReg(100)}, {v: VReg(10).SetRealReg(30)}}
	s.init(activeNodes, instr)
	require.Equal(t, 5, len(s.activeRegs))
	for _, n := range activeNodes {
		if n.r == 30 {
			require.Equal(t, spillHandlerRegState{node: n, state: spillHandlerRegStateBeingUsedNow}, s.activeRegs[n.assignedRealReg()])
		} else {
			require.Equal(t, spillHandlerRegState{node: n, state: spillHandlerRegStateEvictable}, s.activeRegs[n.assignedRealReg()])
		}
	}
}

func TestSpillHandler_getUnusedOrEvictReg(t *testing.T) {
	var s spillHandler
	activeNodes := []*node{{r: RealReg(30)}, {r: RealReg(0)}, {r: RealReg(1)}, {r: RealReg(2)}}
	instr := newMockInstr().use(FromRealReg(30, RegTypeInt))
	s.init(activeNodes, instr)
	require.Equal(t, 4, len(s.activeRegs))

	regInfo := RegisterInfo{
		AllocatableRegisters: [NumRegType][]RealReg{
			RegTypeInt: {
				RealReg(0xff), // unused.
				RealReg(0), RealReg(1), RealReg(2),
			},
			RegTypeFloat: {RealReg(0xaa)},
		},
	}

	// Get unused register.
	r, evicted := s.getUnusedOrEvictReg(RegTypeInt, &regInfo)
	require.Equal(t, RealReg(0xff), r)
	require.Nil(t, evicted)
	require.Equal(t, 5, len(s.activeRegs))
	require.Equal(t, spillHandlerRegState{state: spillHandlerRegStateUsed}, s.activeRegs[RealReg(0xff)])

	// Get unused register.
	r, evicted = s.getUnusedOrEvictReg(RegTypeFloat, &regInfo)
	require.Equal(t, RealReg(0xaa), r)
	require.Nil(t, evicted)
	require.Equal(t, 6, len(s.activeRegs))
	require.Equal(t, spillHandlerRegState{state: spillHandlerRegStateUsed}, s.activeRegs[RealReg(0xaa)])

	// Evict register.
	r, evicted = s.getUnusedOrEvictReg(RegTypeInt, &regInfo)
	require.Equal(t, RealReg(0), r)
	require.Equal(t, activeNodes[1], evicted)
	require.Equal(t, 6, len(s.activeRegs))
	require.Equal(t, spillHandlerRegState{node: activeNodes[1], state: spillHandlerRegStateEvicted}, s.activeRegs[RealReg(0)])

	// Evict again.
	r, evicted = s.getUnusedOrEvictReg(RegTypeInt, &regInfo)
	require.Equal(t, RealReg(1), r)
	require.Equal(t, activeNodes[2], evicted)
	require.Equal(t, 6, len(s.activeRegs))
	require.Equal(t, spillHandlerRegState{node: activeNodes[2], state: spillHandlerRegStateEvicted}, s.activeRegs[RealReg(1)])
}
