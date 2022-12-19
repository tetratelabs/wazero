package asm

// BaseAssemblerImpl includes code common to all architectures.
//
// Note: When possible, add code here instead of in architecture-specific files to reduce drift:
// As this is internal, exporting symbols only to reduce duplication is ok.
type BaseAssemblerImpl struct {
	// SetBranchTargetOnNextNodes holds branch kind instructions (BR, conditional BR, etc.)
	// where we want to set the next coming instruction as the destination of these BR instructions.
	SetBranchTargetOnNextNodes []Node

	JumpTableEntries []JumpTableEntry
}

type JumpTableEntry struct {
	T                        *StaticConst
	LabelInitialInstructions []Node
}

// SetJumpTargetOnNext implements AssemblerBase.SetJumpTargetOnNext
func (a *BaseAssemblerImpl) SetJumpTargetOnNext(nodes ...Node) {
	a.SetBranchTargetOnNextNodes = append(a.SetBranchTargetOnNextNodes, nodes...)
}

// BuildJumpTable implements AssemblerBase.BuildJumpTable
func (a *BaseAssemblerImpl) BuildJumpTable(table *StaticConst, labelInitialInstructions []Node) {
	a.JumpTableEntries = append(a.JumpTableEntries, JumpTableEntry{
		T:                        table,
		LabelInitialInstructions: labelInitialInstructions,
	})
}
