//go:build amd64
// +build amd64

package jit

import (
	"github.com/twitchyliquid64/golang-asm/obj/x86"

	"github.com/tetratelabs/wazero/wasm/wazeroir"
)

type valueLocation struct {
	// TODO: might not be neeeded at all!
	valueType    wazeroir.SignLessType
	register     *int16
	stackPointer *uint64
	// conditional registers?
}

func (v *valueLocation) setStackPointer(sp uint64) {
	v.register = nil
	v.stackPointer = &sp
}

func (v *valueLocation) onStack() bool {
	return v.stackPointer != nil
}

func (v *valueLocation) onRegister() bool {
	return v.register != nil
}

func (v *valueLocation) onConditionalRegister() bool {
	// TODO!
	return false
}

var (
	gpFloatRegisters = []int16{
		x86.REG_X0, x86.REG_X1, x86.REG_X2, x86.REG_X3,
		x86.REG_X4, x86.REG_X5, x86.REG_X6, x86.REG_X7,
		x86.REG_X8, x86.REG_X9, x86.REG_X10, x86.REG_X11,
		x86.REG_X12, x86.REG_X13, x86.REG_X14, x86.REG_X15,
	}
	// Note that we never invoke "call" instruction,
	// so we don't need to care about the calling convension.
	// TODO: we still have to take into acounts RAX,RDX register
	// usages in DIV,MUL operations.
	gpIntRegisters = []int16{
		x86.REG_AX, x86.REG_CX, x86.REG_DX, x86.REG_BX,
		x86.REG_BP, x86.REG_SI, x86.REG_DI, x86.REG_R8,
		x86.REG_R9, x86.REG_R10, x86.REG_R11,
	}
)

func isIntRegister(r int16) bool {
	return gpIntRegisters[0] <= r && r <= gpIntRegisters[len(gpIntRegisters)-1]
}

func isFloatRegister(r int16) bool {
	return gpFloatRegisters[0] <= r && r <= gpFloatRegisters[len(gpFloatRegisters)-1]
}

func newValueLocationStack() *valueLocationStack {
	return &valueLocationStack{
		usedRegisters: map[int16]struct{}{},
	}
}

type valueLocationStack struct {
	stack         []*valueLocation
	sp            int
	usedRegisters map[int16]struct{}
}

func (s *valueLocationStack) push(loc *valueLocation) {
	if s.sp >= len(s.stack) {
		s.stack = append(s.stack, loc)
		s.sp++
	} else {
		s.stack[s.sp] = loc
		s.sp++
	}
}

func (s *valueLocationStack) pop() (loc *valueLocation) {
	s.sp--
	loc = s.stack[s.sp]
	return
}

func (s *valueLocationStack) peek() (loc *valueLocation) {
	loc = s.stack[s.sp-1]
	return
}

func (s *valueLocationStack) releaseRegister(reg int16) {
	delete(s.usedRegisters, reg)
}

func (s *valueLocationStack) markRegisterUsed(reg int16) {
	s.usedRegisters[reg] = struct{}{}
}

type generalPurposeRegisterType byte

const (
	gpTypeInt generalPurposeRegisterType = iota
	gpTypeFloat
)

// Search for unused registers, and if found, returns the resgister
// and mark it used.
func (s *valueLocationStack) takeFreeRegister(tp generalPurposeRegisterType) (reg int16, found bool) {
	var targetRegs []int16
	switch tp {
	case gpTypeFloat:
		targetRegs = gpFloatRegisters
	case gpTypeInt:
		targetRegs = gpIntRegisters
	}
	for _, candidate := range targetRegs {
		if _, ok := s.usedRegisters[candidate]; ok {
			continue
		}
		s.markRegisterUsed(candidate)
		return candidate, true
	}
	return 0, false
}

// Search through the stack, and steal the register from the last used
// variable on the stack.
func (s *valueLocationStack) takeStealTargetFromUsedRegister(tp generalPurposeRegisterType) (*valueLocation, bool) {
	for i := 0; i < s.sp; i++ {
		loc := s.stack[i]
		if loc.onRegister() {
			switch tp {
			case gpTypeFloat:
				if isFloatRegister(*loc.register) {
					return loc, true
				}
			case gpTypeInt:
				if isIntRegister(*loc.register) {
					return loc, true
				}
			}
		}
	}
	return nil, false
}
