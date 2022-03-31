//go:build !debug_asm

package asm_amd64

import "github.com/tetratelabs/wazero/internal/asm"

// NewAssembler implements asm.NewAssembler and is used by default.
// This returns an implementation of Assembler interface via our homemade assembler implementation.
func NewAssembler(temporaryRegister asm.Register) (asm.AssemblerBase, error) {
	a := newAssemblerImpl()
	return a, nil
}
