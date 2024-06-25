package regalloc

import (
	"fmt"
	"strings"
)

// Following mock types are used for testing.
type (
	// mockFunction implements Function.
	mockFunction struct {
		iter            int
		blocks          []*mockBlock
		befores, afters []storeOrReloadInfo
		lnfRoots        []*mockBlock
	}

	storeOrReloadInfo struct {
		reload bool
		v      VReg
		instr  *mockInstr
	}

	// mockBlock implements Block.
	mockBlock struct {
		id             int32
		instructions   []*mockInstr
		preds, succs   []*mockBlock
		_preds, _succs []*mockBlock
		iter           int
		_entry         bool
		_loop          bool
		lnfChildren    []*mockBlock
		blockParams    []VReg
	}

	// mockInstr implements Instr.
	mockInstr struct {
		next, prev                 *mockInstr
		defs, uses                 []VReg
		isCopy, isCall, isIndirect bool
	}
)

func (m *mockFunction) LowestCommonAncestor(blk1, blk2 *mockBlock) *mockBlock { panic("TODO") }

func (m *mockFunction) Idom(blk *mockBlock) *mockBlock { panic("TODO") }

func (m *mockFunction) SwapBefore(x1, x2, tmp VReg, instr *mockInstr) { panic("TODO") }

func (m *mockFunction) InsertMoveBefore(dst, src VReg, instr *mockInstr) { panic("TODO") }

func newMockFunction(blocks ...*mockBlock) *mockFunction {
	return &mockFunction{blocks: blocks}
}

func (m *mockFunction) loopNestingForestRoots(blocks ...*mockBlock) {
	m.lnfRoots = blocks
}

func newMockBlock(id int32, instructions ...*mockInstr) *mockBlock {
	if len(instructions) > 0 {
		instructions[0].prev = nil
		for i := 1; i < len(instructions); i++ {
			instructions[i].prev = instructions[i-1]
			instructions[i-1].next = instructions[i]
		}
		instructions[len(instructions)-1].next = nil
	}
	return &mockBlock{id: id, instructions: instructions}
}

func newMockInstr() *mockInstr {
	return &mockInstr{}
}

// String implements fmt.Stringer for debugging.
func (m *mockFunction) String() string {
	var block []string
	for _, b := range m.blocks {
		block = append(block, "\t"+b.String())
	}
	return fmt.Sprintf("mockFunction:\n%s", strings.Join(block, ",\n"))
}

// String implements fmt.Stringer for debugging.
func (m *mockInstr) String() string {
	return fmt.Sprintf("mockInstr{defs=%v, uses=%v}", m.defs, m.uses)
}

// String implements fmt.Stringer for debugging.
func (m *mockBlock) String() string {
	var preds []int32
	for _, p := range m.preds {
		preds = append(preds, p.id)
	}
	return fmt.Sprintf("mockBlock{\n\tid=%v,\n\tinstructions=%v,\n\tpreds=%v,\n}", m.id, preds, m.instructions)
}

func (m *mockBlock) addPred(b *mockBlock) {
	m.preds = append(m.preds, b)
	m._preds = append(m._preds, b)
	b._succs = append(b._succs, m)
	b.succs = append(b.succs, m)
}

func (m *mockInstr) use(uses ...VReg) *mockInstr {
	m.uses = uses
	return m
}

func (m *mockInstr) def(defs ...VReg) *mockInstr {
	m.defs = defs
	return m
}

func (m *mockBlock) loop(children ...*mockBlock) *mockBlock {
	m._loop = true
	m.lnfChildren = children
	return m
}

func (m *mockBlock) entry() *mockBlock {
	m._entry = true
	return m
}

func (m *mockInstr) asCopy() *mockInstr {
	m.isCopy = true
	return m
}

func (m *mockInstr) asCall() *mockInstr { //nolint:unused
	m.isCall = true
	return m
}

func (m *mockInstr) asIndirectCall() *mockInstr { //nolint:unused
	m.isIndirect = true
	return m
}

// StoreRegisterAfter implements Function.StoreRegisterAfter.
func (m *mockFunction) StoreRegisterAfter(v VReg, instr *mockInstr) {
	m.afters = append(m.afters, storeOrReloadInfo{false, v, instr})
}

// ReloadRegisterBefore implements Function.ReloadRegisterBefore.
func (m *mockFunction) ReloadRegisterBefore(v VReg, instr *mockInstr) {
	m.befores = append(m.befores, storeOrReloadInfo{true, v, instr})
}

// StoreRegisterBefore implements Function.StoreRegisterBefore.
func (m *mockFunction) StoreRegisterBefore(v VReg, instr *mockInstr) {
	m.befores = append(m.befores, storeOrReloadInfo{false, v, instr})
}

// ReloadRegisterAfter implements Function.ReloadRegisterAfter.
func (m *mockFunction) ReloadRegisterAfter(v VReg, instr *mockInstr) {
	m.afters = append(m.afters, storeOrReloadInfo{true, v, instr})
}

// ClobberedRegisters implements Function.ClobberedRegisters.
func (m *mockFunction) ClobberedRegisters(regs []VReg) {
	// TODO implement me
	panic("implement me")
}

// Done implements Function.Done.
func (m *mockFunction) Done() {}

// PostOrderBlockIteratorBegin implements Block.
func (m *mockFunction) PostOrderBlockIteratorBegin() *mockBlock {
	m.iter = 1
	l := len(m.blocks)
	return m.blocks[l-1]
}

// PostOrderBlockIteratorNext implements Block.
func (m *mockFunction) PostOrderBlockIteratorNext() *mockBlock {
	if m.iter == len(m.blocks) {
		return nil
	}
	l := len(m.blocks)
	ret := m.blocks[l-m.iter-1]
	m.iter++
	return ret
}

// ReversePostOrderBlockIteratorBegin implements Block.
func (m *mockFunction) ReversePostOrderBlockIteratorBegin() *mockBlock {
	m.iter = 1
	return m.blocks[0]
}

// ReversePostOrderBlockIteratorNext implements Block.
func (m *mockFunction) ReversePostOrderBlockIteratorNext() *mockBlock {
	if m.iter == len(m.blocks) {
		return nil
	}
	ret := m.blocks[m.iter]
	m.iter++
	return ret
}

// ID implements Block.
func (m *mockBlock) ID() int32 {
	return m.id
}

// InstrIteratorBegin implements Block.
func (m *mockBlock) InstrIteratorBegin() *mockInstr {
	if len(m.instructions) == 0 {
		return nil
	}
	m.iter = 1
	return m.instructions[0]
}

// InstrIteratorNext implements Block.
func (m *mockBlock) InstrIteratorNext() *mockInstr {
	if m.iter == len(m.instructions) {
		return nil
	}
	ret := m.instructions[m.iter]
	m.iter++
	return ret
}

// InstrRevIteratorBegin implements Block.
func (m *mockBlock) InstrRevIteratorBegin() *mockInstr {
	if len(m.instructions) == 0 {
		return nil
	}
	m.iter = len(m.instructions)
	return m.InstrRevIteratorNext()
}

// InstrRevIteratorNext implements Block.
func (m *mockBlock) InstrRevIteratorNext() *mockInstr {
	m.iter--
	if m.iter < 0 {
		return nil
	}
	return m.instructions[m.iter]
}

// Preds implements Block.
func (m *mockBlock) Preds() int {
	return len(m._preds)
}

// BlockParams implements Block.
func (m *mockFunction) BlockParams(blk *mockBlock, ret *[]VReg) []VReg {
	*ret = append((*ret)[:0], blk.blockParams...)
	return *ret
}

func (m *mockBlock) blockParam(v VReg) {
	m.blockParams = append(m.blockParams, v)
}

// Defs implements Instr.
func (m *mockInstr) Defs(ret *[]VReg) []VReg {
	*ret = append((*ret)[:0], m.defs...)
	return *ret
}

// Uses implements Instr.
func (m *mockInstr) Uses(ret *[]VReg) []VReg {
	*ret = append((*ret)[:0], m.uses...)
	return *ret
}

// IsCopy implements Instr.
func (m *mockInstr) IsCopy() bool { return m.isCopy }

// IsCall implements Instr.
func (m *mockInstr) IsCall() bool { return m.isCall }

// IsIndirectCall implements Instr.
func (m *mockInstr) IsIndirectCall() bool { return m.isIndirect }

// IsReturn implements Instr.
func (m *mockInstr) IsReturn() bool { return false }

// Next implements Instr.
func (m *mockInstr) Next() *mockInstr { return m.next }

// Prev implements Instr.
func (m *mockInstr) Prev() *mockInstr { return m.prev }

// Entry implements Entry.
func (m *mockBlock) Entry() bool { return m._entry }

// AssignUses implements Instr.
func (m *mockInstr) AssignUse(index int, reg VReg) {
	if index >= len(m.uses) {
		m.uses = append(m.uses, make([]VReg, 5)...)
	}
	m.uses[index] = reg
}

// AssignDef implements Instr.
func (m *mockInstr) AssignDef(reg VReg) {
	m.defs = []VReg{reg}
}

var _ Function[*mockInstr, *mockBlock] = (*mockFunction)(nil)

func (m *mockFunction) LoopNestingForestRoots() int {
	return len(m.lnfRoots)
}

// LoopNestingForestRoot implements Function.
func (m *mockFunction) LoopNestingForestRoot(i int) *mockBlock {
	return m.lnfRoots[i]
}

// LoopNestingForestChild implements Function.
func (m *mockFunction) LoopNestingForestChild(b *mockBlock, i int) *mockBlock {
	return b.lnfChildren[i]
}

// Succ implements Function.
func (m *mockFunction) Succ(b *mockBlock, i int) *mockBlock {
	return b.succs[i]
}

// Pred implements Function.
func (m *mockFunction) Pred(b *mockBlock, i int) *mockBlock { return b._preds[i] }

func (m *mockBlock) LoopHeader() bool {
	return m._loop
}

func (m *mockBlock) Succs() int {
	return len(m.succs)
}

func (m *mockBlock) LoopNestingForestChildren() int {
	return len(m.lnfChildren)
}

func (m *mockBlock) LastInstrForInsertion() *mockInstr {
	if len(m.instructions) == 0 {
		return nil
	}
	return m.instructions[len(m.instructions)-1]
}

func (m *mockBlock) FirstInstr() *mockInstr {
	return m.instructions[0]
}
