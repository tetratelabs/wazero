package asm

import "fmt"

// Register represents architecture-specific registers.
type Register byte

// NilRegister is the only architecture-independent register, and
// can be used to indicate that no register is specified.
const NilRegister Register = 0

// Instruction represents architecture-specific instructions.
type Instruction byte

// ConditionalRegisterState represents architecture-specific conditional
// register's states.
type ConditionalRegisterState byte

// NilRegister is the only architecture-independent conditinal state, and
// can be used to indicate that no conditional state is specificed.
const ConditionalRegisterStateUnset ConditionalRegisterState = 0

// Node represents a node in the linked list of assembled operations.
type Node interface {
	fmt.Stringer
	// AssignJumpTarget assigns the given target node as the destination of
	// jump instruction for this Node.
	AssignJumpTarget(target Node)
	// AssignDestinationConstant assigns the given constnat as the destination
	// of the instruction for this node.
	AssignDestinationConstant(value int64)
	// AssignSourceConstant assigns the given constnat as the source
	// of the instruction for this node.
	AssignSourceConstant(value int64)
	// OffsetInBinary returns the offset of this node in the assembled binary.
	OffsetInBinary() int64
}

// AssemblerBase is the common interface for assemblers among multiple architectures.
//
// Note: some of them can be implemented in a arch-independent way, but not all can be
// implemented as such. However, we intentionally put such arch-dependant methods here
// in order to provide the common documentation interface.
type AssemblerBase interface {
	// Assemble produces the final binary for the assembled operations.
	Assemble() ([]byte, error)
	// SetJumpTargetOnNext instructs the assmembler that the next node must be
	// assigned to the given nodes's jump destination.
	SetJumpTargetOnNext(nodes ...Node)
	// BuildJumpTable calculates the offsets between the first instruction `initialInstructions[0]`
	// and others (e.g. initialInstructions[3]), and wrote the calcualted offsets into pre-allocated
	// `table` slice in litte endian.
	//
	// TODO: This can be hidden into assembler implementation after golang-asm removal.
	BuildJumpTable(table []byte, initialInstructions []Node)
	// CompileStandAlone adds an instruction to take no arguments.
	CompileStandAlone(instruction Instruction) Node
	// CompileConstToRegister adds an instruction where source operand is `value` as constant and destination is `destinationReg` register.
	CompileConstToRegister(instruction Instruction, value int64, destinationReg Register) Node
	// CompileConstToRegister adds an instruction where source and destination operands are registers.
	CompileRegisterToRegister(instruction Instruction, from, to Register)
	// CompileMemoryToRegister adds an instruction where source operands is the memory address specified by `sourceBaseReg+sourceOffsetConst`
	// and the destination is `destinationReg` register.
	CompileMemoryToRegister(instruction Instruction, sourceBaseReg Register, sourceOffsetConst int64, destinationReg Register)
	// CompileRegisterToMemory adds an instruction where source operand is `sourceRegister` register and the destination is the
	// memory address specified by `destinationBaseRegister+destinationOffsetConst`.
	CompileRegisterToMemory(instruction Instruction, sourceRegister Register, destinationBaseRegister Register, destinationOffsetConst int64)
	// CompileJump adds jump-type instruction and returns the corresponding Node in the assembled linked list.
	CompileJump(jmpInstruction Instruction) Node
	// CompileJumpToMemory adds jump-type instruction whose destination is stored in the memory address specified by `baseReg+offset`,
	// and returns the corresponding Node in the assembled linked list.
	CompileJumpToMemory(jmpInstruction Instruction, baseReg Register, offset int64)
	// CompileJumpToMemory adds jump-type instruction whose destination is the memory address specified by `reg` register.
	CompileJumpToRegister(jmpInstruction Instruction, reg Register)
	// CompileReadInstructionAddress adds an ADR instruction to set the absolute address of "target instruction"
	// into destinationRegister. "target instruction" is specified by beforeTargetInst argument and
	// the target is determined by "the instruction right after beforeTargetInst type".
	//
	// For example, if beforeTargetInst == RET and we have the instruction sequence like
	// ADR -> X -> Y -> ... -> RET -> MOV, then the ADR instruction emitted by this function set the absolute
	// address of MOV instruction into the destination register.
	CompileReadInstructionAddress(destinationRegister Register, beforeAcquisitionTargetInstruction Instruction)
}
