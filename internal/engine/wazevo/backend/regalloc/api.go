package regalloc

import "fmt"

// These interfaces are implemented by ISA-specific backends to abstract away the details, and allow the register
// allocators to work on any ISA.
//
// TODO: the interfaces are not stabilized yet, especially x64 will need some changes. E.g. x64 has an addressing mode
// 	where index can be in memory. That kind of info will be useful to reduce the register pressure, and should be leveraged
// 	by the register allocators, like https://docs.rs/regalloc2/latest/regalloc2/enum.OperandConstraint.html

type (
	// Function is the top-level interface to do register allocation, which corresponds to a CFG containing
	// Blocks(s).
	Function interface {
		// PostOrderBlockIteratorBegin returns the first block in the post-order traversal of the CFG.
		// In other words, the last blocks in the CFG will be returned first.
		PostOrderBlockIteratorBegin() Block
		// PostOrderBlockIteratorNext returns the next block in the post-order traversal of the CFG.
		PostOrderBlockIteratorNext() Block
		// ReversePostOrderBlockIteratorBegin returns the first block in the reverse post-order traversal of the CFG.
		// In other words, the first blocks in the CFG will be returned first.
		ReversePostOrderBlockIteratorBegin() Block
		// ReversePostOrderBlockIteratorNext returns the next block in the reverse post-order traversal of the CFG.
		ReversePostOrderBlockIteratorNext() Block
		// ClobberedRegisters tell the clobbered registers by this function.
		ClobberedRegisters([]VReg)
		// StoreRegisterBefore ... TODO
		StoreRegisterBefore(v VReg, instr Instr)
		// StoreRegisterAfter ... TODO
		StoreRegisterAfter(v VReg, instr Instr)
		// ReloadRegisterBefore ... TODO
		ReloadRegisterBefore(v VReg, instr Instr)
		// ReloadRegisterAfter ... TODO
		ReloadRegisterAfter(v VReg, instr Instr)
		// Done tells the implementation that register allocation is done, and it can finalize the stack
		Done()
	}

	// Block is a basic block in the CFG of a function, and it consists of multiple instructions, and predecessor Block(s).
	Block interface {
		// ID returns the unique identifier of this block.
		ID() int
		// InstrIteratorBegin returns the first instruction in this block. Instructions added after lowering must be skipped.
		// Note: multiple Instr(s) will not be held at the same time, so it's safe to use the same impl for the return Instr.
		InstrIteratorBegin() Instr
		// InstrIteratorNext returns the next instruction in this block. Instructions added after lowering must be skipped.
		// Note: multiple Instr(s) will not be held at the same time, so it's safe to use the same impl for the return Instr.
		InstrIteratorNext() Instr
		// Preds returns the predecessors of this block in the CFG.
		// Note: multiple returned []Block will not be used at the same time, so it's safe to use the same slice for []Block.
		Preds() []Block
		// Entry returns true if the block is for the entry block.
		Entry() bool
	}

	// Instr is an instruction in a block, abstracting away the underlying ISA.
	Instr interface {
		fmt.Stringer

		// Defs returns the virtual registers defined by this instruction.
		// Note: multiple returned []VReg will not be held at the same time, so it's safe to use the same slice for this.
		Defs() []VReg
		// Uses returns the virtual registers used by this instruction.
		// Note: multiple returned []VReg will not be held at the same time, so it's safe to use the same slice for this.
		Uses() []VReg
		// AssignUses assigns the RealReg-allocated virtual registers used by this instruction.
		// Note: input []VReg is reused, so it's not safe to hold reference to it after the end of this call.
		AssignUses([]VReg)
		// AssignDef assigns a RealReg-allocated virtual register defined by this instruction.
		// This only accepts one register because we don't allocate registers for multi-def instructions (i.e. call instruction)
		AssignDef(VReg)
		// IsCopy returns true if this instruction is a move instruction between two registers.
		// If true, the instruction is of the form of dst = src, and if the src and dst do not interfere with each other,
		// we could coalesce them, and hence the copy can be eliminated from the final code.
		IsCopy() bool
		// IsCall returns true if this instruction is a call instruction. The result is used to insert
		// caller saved register spills and restores.
		IsCall() bool
		// IsIndirectCall returns true if this instruction is an indirect call instruction.
		IsIndirectCall() bool
		// IsReturn returns true if this instruction is a return instruction.
		IsReturn() bool
	}
)
