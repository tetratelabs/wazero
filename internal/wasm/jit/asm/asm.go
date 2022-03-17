package asm

// TODO: we don't need 16 bits for register representations,
// so change this to byte type after removing golang-asm
type Register int16

const NilRegister Register = 0

type Instruction int16

type ConditionalRegisterState byte

const ConditionalRegisterStateUnset ConditionalRegisterState = 0

type Node interface {
	Pc() int64
	AssignJumpTarget(target Node)
	AssignDestinationConstant(value int64)
	AssignSourceConstant(value int64)
}

type AssemblerBase interface {
	Assemble() ([]byte, error)
	SetBranchTargetOnNext(nodes ...Node)
	// TODO
	BuildJumpTable(table []byte, initialInstructions []Node)
}
