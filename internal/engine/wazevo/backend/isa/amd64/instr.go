package amd64

import (
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
)

type instruction struct {
	kind                instructionKind
	prev, next          *instruction
	addedBeforeRegAlloc bool
}

// Next implements regalloc.Instr.
func (i *instruction) Next() regalloc.Instr {
	return i.next
}

// Prev implements regalloc.Instr.
func (i *instruction) Prev() regalloc.Instr {
	return i.prev
}

// String implements regalloc.Instr.
func (i *instruction) String() string {
	// TODO implement me
	panic("implement me")
}

// Defs implements regalloc.Instr.
func (i *instruction) Defs(i2 *[]regalloc.VReg) []regalloc.VReg {
	// TODO implement me
	panic("implement me")
}

// Uses implements regalloc.Instr.
func (i *instruction) Uses(i2 *[]regalloc.VReg) []regalloc.VReg {
	// TODO implement me
	panic("implement me")
}

// AssignUse implements regalloc.Instr.
func (i *instruction) AssignUse(index int, v regalloc.VReg) {
	// TODO implement me
	panic("implement me")
}

// AssignDef implements regalloc.Instr.
func (i *instruction) AssignDef(reg regalloc.VReg) {
	// TODO implement me
	panic("implement me")
}

// IsCopy implements regalloc.Instr.
func (i *instruction) IsCopy() bool {
	// TODO implement me
	panic("implement me")
}

// IsCall implements regalloc.Instr.
func (i *instruction) IsCall() bool {
	// TODO implement me
	panic("implement me")
}

// IsIndirectCall implements regalloc.Instr.
func (i *instruction) IsIndirectCall() bool {
	// TODO implement me
	panic("implement me")
}

// IsReturn implements regalloc.Instr.
func (i *instruction) IsReturn() bool {
	// TODO implement me
	panic("implement me")
}

// AddedBeforeRegAlloc implements regalloc.Instr.
func (i *instruction) AddedBeforeRegAlloc() bool {
	// TODO implement me
	panic("implement me")
}

func resetInstruction(i *instruction) {
	*i = instruction{}
}

func setNext(i *instruction, next *instruction) {
	i.next = next
}

func setPrev(i *instruction, prev *instruction) {
	i.prev = prev
}

func asNop(i *instruction) {
	i.kind = nop0
}

type instructionKind int

const (
	nop0 instructionKind = iota + 1
)
