package amd64

import (
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

// lowerToAddressMode converts a pointer to an addressMode that can be used as an operand for load/store instructions.
func (m *machine) lowerToAddressMode(ptr ssa.Value, offsetBase uint32) (am amode) {
	panic("TODO")
}
