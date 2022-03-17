package asm

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
	// AssignJumpTarget assigns the given target node as the destination of
	// jump instruction for this Node.
	AssignJumpTarget(target Node)
	// AssignDestinationConstant assigns the given constnat as the destination
	// of the instruction for this node.
	AssignDestinationConstant(value int64)
	// AssignSourceConstant assigns the given constnat as the source
	// of the instruction for this node.
	AssignSourceConstant(value int64)
	// offsetInBinary returns the offset of this node in the assembled binary.
	offsetInBinary() int64
}

// AssemblerBase is the common interface for assemblers among multiple architectures.
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
	// TODO
	CompileStandAlone(instruction Instruction) Node
	// TODO
	CompileConstToRegister(instruction Instruction, constValue int64, destinationReg Register) (inst Node)
	// TODO
	CompileRegisterToRegister(instruction Instruction, from, to Register)
	// TODO
	CompileMemoryToRegister(instruction Instruction, sourceBaseReg Register, sourceOffsetConst int64, destinationReg Register)
	// TODO
	CompileRegisterToMemory(inst Instruction, sourceRegister Register, destinationBaseRegister Register, destinationOffsetConst int64)
	// TODO
	CompileJump(jmpInstruction Instruction) Node
	// TODO
	CompileJumpToMemory(jmpInstruction Instruction, baseReg Register, offset int64)
	// TODO
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
