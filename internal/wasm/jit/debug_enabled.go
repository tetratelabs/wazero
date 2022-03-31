//go:build debug_asm

package jit

import (
	"github.com/tetratelabs/wazero/internal/asm"
	asm_amd64 "github.com/tetratelabs/wazero/internal/asm/amd64"
	"github.com/tetratelabs/wazero/internal/asm/amd64_debug"
)

// newAmd64Assembler implements asm.NewAssembler and is used when the "debug_asm" flag is on.
func newAmd64Assembler(temporaryRegister asm.Register) (asm_amd64.Assembler, error) {
	return amd64_debug.NewDebugAssembler()
}
