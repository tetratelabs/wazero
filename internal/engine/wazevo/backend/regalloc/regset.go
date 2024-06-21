package regalloc

import (
	"fmt"
	"strings"
)

// NewRegSet returns a new RegSet with the given registers.
func NewRegSet(regs ...RealReg) RegSet {
	var ret RegSet
	for _, r := range regs {
		ret = ret.add(r)
	}
	return ret
}

// RegSet represents a set of registers.
type RegSet uint64

func (rs RegSet) format(info *RegisterInfo) string { //nolint:unused
	var ret []string
	for i := 0; i < 64; i++ {
		if rs&(1<<uint(i)) != 0 {
			ret = append(ret, info.RealRegName(RealReg(i)))
		}
	}
	return strings.Join(ret, ", ")
}

func (rs RegSet) has(r RealReg) bool {
	return rs&(1<<uint(r)) != 0
}

func (rs RegSet) add(r RealReg) RegSet {
	if r >= 64 {
		return rs
	}
	return rs | 1<<uint(r)
}

func (rs RegSet) Range(f func(allocatedRealReg RealReg)) {
	for i := 0; i < 64; i++ {
		if rs&(1<<uint(i)) != 0 {
			f(RealReg(i))
		}
	}
}

type regInUseSet [64]*vrState

func newRegInUseSet() regInUseSet {
	var ret regInUseSet
	ret.reset()
	return ret
}

func (rs *regInUseSet) reset() {
	for i := range rs {
		rs[i] = nil
	}
}

func (rs *regInUseSet) format(info *RegisterInfo) string { //nolint:unused
	var ret []string
	for i, vr := range rs {
		if vr != nil {
			ret = append(ret, fmt.Sprintf("(%s->v%d)", info.RealRegName(RealReg(i)), vr.v.ID()))
		}
	}
	return strings.Join(ret, ", ")
}

func (rs *regInUseSet) has(r RealReg) bool {
	return r < 64 && rs[r] != nil
}

func (rs *regInUseSet) get(r RealReg) *vrState {
	return rs[r]
}

func (rs *regInUseSet) remove(r RealReg) {
	rs[r] = nil
}

func (rs *regInUseSet) add(r RealReg, vr *vrState) {
	if r >= 64 {
		return
	}
	rs[r] = vr
}

func (rs *regInUseSet) range_(f func(allocatedRealReg RealReg, vr *vrState)) {
	for i, vr := range rs {
		if vr != nil {
			f(RealReg(i), vr)
		}
	}
}
