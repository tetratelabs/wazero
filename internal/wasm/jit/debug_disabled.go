//go:build !debug_asm

package jit

import (
	"github.com/tetratelabs/wazero/internal/asm"
	asm_amd64 "github.com/tetratelabs/wazero/internal/asm/amd64"
)

// newAmd64Assembler implements asm.NewAssembler and is used by default.
// This returns an implementation of Assembler interface via our homemade assembler implementation.
func newAmd64Assembler(temporaryRegister asm.Register) (asm.AssemblerBase, error) {
	a := asm_amd64.NewAssemblerImpl()
	return a, nil
}
