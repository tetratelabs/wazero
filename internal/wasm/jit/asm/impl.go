package asm

import (
	"encoding/binary"
	"fmt"
	"math"
)

type BaseAssemblerImpl struct {
	// setBranchTargetOnNextInstructions holds branch kind instructions (BR, conditional BR, etc)
	// where we want to set the next coming instruction as the destination of these BR instructions.
	setBranchTargetOnNextNodes []Node
	// onGenerateCallbacks holds the callbacks which are called after generating native code.
	OnGenerateCallbacks []func(code []byte) error
}

// SetJumpTargetOnNext implements AssemblerBase.SetJumpTargetOnNext
func (a *BaseAssemblerImpl) SetJumpTargetOnNext(nodes ...Node) {
	a.setBranchTargetOnNextNodes = append(a.setBranchTargetOnNextNodes, nodes...)
}

// AddOnGenerateCallBack implements AssemblerBase.AddOnGenerateCallBack
func (a *BaseAssemblerImpl) AddOnGenerateCallBack(cb func([]byte) error) {
	a.OnGenerateCallbacks = append(a.OnGenerateCallbacks, cb)
}

// BuildJumpTable implements AssemblerBase.BuildJumpTable
func (a *BaseAssemblerImpl) BuildJumpTable(table []byte, labelInitialInstructions []Node) {
	a.AddOnGenerateCallBack(func(code []byte) error {
		// Build the offset table for each target including default one.
		base := labelInitialInstructions[0].OffsetInBinary() // This corresponds to the L0's address in the example.
		for i, nop := range labelInitialInstructions {
			if uint64(nop.OffsetInBinary())-uint64(base) >= math.MaxUint32 {
				// TODO: this happens when users try loading an extremely large webassembly binary
				// which contains a br_table statement with approximately 4294967296 (2^32) targets.
				// We would like to support that binary, but realistically speaking, that kind of binary
				// could result in more than ten giga bytes of native JITed code where we have to care about
				// huge stacks whose height might exceed 32-bit range, and such huge stack doesn't work with the
				// current implementation.
				return fmt.Errorf("too large br_table")
			}
			// We store the offset from the beginning of the L0's initial instruction.
			binary.LittleEndian.PutUint32(table[i*4:(i+1)*4], uint32(nop.OffsetInBinary())-uint32(base))
		}
		return nil
	})
}
