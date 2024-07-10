package backend

import (
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

// SSAValueDefinition represents a definition of an SSA value.
// TODO: this eventually should be deleted.
type SSAValueDefinition struct {
	V ssa.Value
	// Instr is not nil if this is a definition from an instruction.
	Instr *ssa.Instruction
	// RefCount is the number of references to the result.
	RefCount uint32
}

func (d *SSAValueDefinition) IsFromInstr() bool {
	return d.Instr != nil
}
