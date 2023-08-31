package regalloc

import (
	"fmt"
	"strings"
)

// Following mock types are used for testing.
type (
	// mockFunction implements Function.
	mockFunction struct {
		iter   int
		blocks []*mockBlock
		storeRegisterAfter, reloadRegisterAfter,
		storeRegisterBefore, reloadRegisterBefore []storeOrReloadInfo
	}

	storeOrReloadInfo struct {
		v     VReg
		instr Instr
	}

	// mockBlock implements Block.
	mockBlock struct {
		id           int
		instructions []*mockInstr
		preds        []*mockBlock
		_preds       []Block
		iter         int
		_entry       bool
	}

	// mockInstr implements Instr.
	mockInstr struct {
		defs, uses                 []VReg
		isCopy, isCall, isIndirect bool
	}
)

func newMockFunction(blocks ...*mockBlock) *mockFunction {
	return &mockFunction{blocks: blocks}
}

func newMockBlock(id int, instructions ...*mockInstr) *mockBlock {
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
	var preds []int
	for _, p := range m.preds {
		preds = append(preds, p.id)
	}
	return fmt.Sprintf("mockBlock{\n\tid=%v,\n\tinstructions=%v,\n\tpreds=%v,\n}", m.id, preds, m.instructions)
}

func (m *mockBlock) addPred(b *mockBlock) {
	m.preds = append(m.preds, b)
	m._preds = append(m._preds, b)
}

func (m *mockInstr) use(uses ...VReg) *mockInstr {
	m.uses = uses
	return m
}

func (m *mockInstr) def(defs ...VReg) *mockInstr {
	m.defs = defs
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

func (m *mockInstr) asCall() *mockInstr {
	m.isCall = true
	return m
}

func (m *mockInstr) asIndirectCall() *mockInstr {
	m.isIndirect = true
	return m
}

// StoreRegisterAfter implements Function.StoreRegisterAfter.
func (m *mockFunction) StoreRegisterAfter(v VReg, instr Instr) {
	m.storeRegisterAfter = append(m.storeRegisterAfter, storeOrReloadInfo{v, instr})
}

// ReloadRegisterBefore implements Function.ReloadRegisterBefore.
func (m *mockFunction) ReloadRegisterBefore(v VReg, instr Instr) {
	m.reloadRegisterBefore = append(m.reloadRegisterBefore, storeOrReloadInfo{v, instr})
}

// StoreRegisterBefore implements Function.StoreRegisterBefore.
func (m *mockFunction) StoreRegisterBefore(v VReg, instr Instr) {
	m.storeRegisterBefore = append(m.storeRegisterBefore, storeOrReloadInfo{v, instr})
}

// ReloadRegisterAfter implements Function.ReloadRegisterAfter.
func (m *mockFunction) ReloadRegisterAfter(v VReg, instr Instr) {
	m.reloadRegisterAfter = append(m.reloadRegisterAfter, storeOrReloadInfo{v, instr})
}

// ClobberedRegisters implements Function.ClobberedRegisters.
func (m *mockFunction) ClobberedRegisters(regs []VReg) {
	// TODO implement me
	panic("implement me")
}

// Done implements Function.Done.
func (m *mockFunction) Done() {}

// PostOrderBlockIteratorBegin implements Block.
func (m *mockFunction) PostOrderBlockIteratorBegin() Block {
	m.iter = 1
	l := len(m.blocks)
	return m.blocks[l-1]
}

// PostOrderBlockIteratorNext implements Block.
func (m *mockFunction) PostOrderBlockIteratorNext() Block {
	if m.iter == len(m.blocks) {
		return nil
	}
	l := len(m.blocks)
	ret := m.blocks[l-m.iter-1]
	m.iter++
	return ret
}

// ReversePostOrderBlockIteratorBegin implements Block.
func (m *mockFunction) ReversePostOrderBlockIteratorBegin() Block {
	m.iter = 1
	return m.blocks[0]
}

// ReversePostOrderBlockIteratorNext implements Block.
func (m *mockFunction) ReversePostOrderBlockIteratorNext() Block {
	if m.iter == len(m.blocks) {
		return nil
	}
	ret := m.blocks[m.iter]
	m.iter++
	return ret
}

// ID implements Block.
func (m *mockBlock) ID() int {
	return m.id
}

// InstrIteratorBegin implements Block.
func (m *mockBlock) InstrIteratorBegin() Instr {
	if len(m.instructions) == 0 {
		return nil
	}
	m.iter = 1
	return m.instructions[0]
}

// InstrIteratorNext implements Block.
func (m *mockBlock) InstrIteratorNext() Instr {
	if m.iter == len(m.instructions) {
		return nil
	}
	ret := m.instructions[m.iter]
	m.iter++
	return ret
}

// Preds implements Instr.
func (m *mockBlock) Preds() []Block {
	return m._preds
}

// Defs implements Instr.
func (m *mockInstr) Defs() []VReg {
	return m.defs
}

// Uses implements Instr.
func (m *mockInstr) Uses() []VReg {
	return m.uses
}

// IsCopy implements Instr.
func (m *mockInstr) IsCopy() bool { return m.isCopy }

// IsCall implements Instr.
func (m *mockInstr) IsCall() bool { return m.isCall }

// IsIndirectCall implements Instr.
func (m *mockInstr) IsIndirectCall() bool { return m.isIndirect }

// IsReturn implements Instr.
func (m *mockInstr) IsReturn() bool { return false }

// Entry implements Entry.
func (m *mockBlock) Entry() bool { return m._entry }

// AssignUses implements Instr.
func (m *mockInstr) AssignUses(regs []VReg) {
	m.uses = make([]VReg, len(regs))
	copy(m.uses, regs)
}

// AssignDef implements Instr.
func (m *mockInstr) AssignDef(reg VReg) {
	m.defs = []VReg{reg}
}

var (
	_ Function = (*mockFunction)(nil)
	_ Block    = (*mockBlock)(nil)
	_ Instr    = (*mockInstr)(nil)
)
