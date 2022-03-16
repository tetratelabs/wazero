package asm

// Alias, not type definition for convenience.
// TODO: we don't need 16 bits for register representations,
// so change this to byte type after removing golang-asm
type Register = int16

// Alias, not type definition for convenience.
type Instruction = int16

type Node interface {
	Pc() int64
	AssignJumpTarget(target Node)
	AssignDestinationConstant(value int64)
}

type AssemblerBase interface {
	Assemble() []byte
	SetBranchTargetOnNext(nodes ...Node)
}
