package regalloc

// spillHandler is a helper to handle the spill and reload of registers at some point in the program.
type spillHandler struct {
	activeRegs   map[RealReg]spillHandlerRegState
	deleteTemp   []RealReg
	beingUsedNow map[RealReg]struct{}
}

type spillHandlerRegState struct {
	state int
	node  *node
}

const (
	spillHandlerRegStateUsed = iota
	spillHandlerRegStateEvictable
	spillHandlerRegStateEvicted
	spillHandlerRegStateBeingUsedNow
)

// init initializes the spill handler with the active nodes which are the node alive at a some point in the program.
func (s *spillHandler) init(activeNodes []*node, inst Instr) {
	if s.beingUsedNow == nil {
		s.beingUsedNow = make(map[RealReg]struct{})
	} else {
		s.deleteTemp = s.deleteTemp[:0]
		for r := range s.beingUsedNow {
			s.deleteTemp = append(s.deleteTemp, r)
		}
		for _, r := range s.deleteTemp {
			delete(s.beingUsedNow, r)
		}
		for _, u := range inst.Uses() {
			if u.IsRealReg() {
				s.beingUsedNow[u.RealReg()] = struct{}{}
			}
		}
	}

	if s.activeRegs == nil {
		s.activeRegs = make(map[RealReg]spillHandlerRegState)
	} else {
		s.deleteTemp = s.deleteTemp[:0]
		for r := range s.activeRegs {
			s.deleteTemp = append(s.deleteTemp, r)
		}
		for _, r := range s.deleteTemp {
			delete(s.activeRegs, r)
		}
	}
	for _, n := range activeNodes {
		r := n.assignedRealReg()
		if _, ok := s.beingUsedNow[r]; ok {
			s.activeRegs[r] = spillHandlerRegState{node: n, state: spillHandlerRegStateBeingUsedNow}
		} else {
			s.activeRegs[r] = spillHandlerRegState{node: n, state: spillHandlerRegStateEvictable}
		}
	}
}

// getUnusedOrEvictReg returns an unused register of the given type or evicts a register from the active registers.
func (s *spillHandler) getUnusedOrEvictReg(regType RegType, regInfo *RegisterInfo) (r RealReg, evicted *node) {
	allocatables := regInfo.AllocatableRegisters[regType]
	for _, candidate := range allocatables {
		_, ok := s.activeRegs[candidate]
		if !ok {
			r = candidate
			s.activeRegs[candidate] = spillHandlerRegState{state: spillHandlerRegStateUsed}
			break
		} // ok=true meaning that it is either used or evicted.
	}

	if r == RealRegInvalid {
		// We need to evict a register from the active registers.
		for _, candidate := range allocatables {
			state, ok := s.activeRegs[candidate]
			if !ok {
				panic("BUG")
			}
			if state.state == spillHandlerRegStateEvictable {
				evicted = state.node
				r = candidate
				s.activeRegs[candidate] = spillHandlerRegState{node: state.node, state: spillHandlerRegStateEvicted}
				break
			}
		}
	}
	return
}
