//go:build amd64
// +build amd64

package jit

import (
	"errors"

	"github.com/twitchyliquid64/golang-asm/obj/x86"
)

type valueLocation struct {
	register   *int
	stackDepth *int
	// conditional registers?
}

func (v *valueLocation) onStack() bool {
	return v.stackDepth != nil
}

func (v *valueLocation) onRegister() bool {
	return v.register != nil
}

var (
	gpFloatRegisters = []int{
		x86.REG_X0, x86.REG_X1, x86.REG_X2, x86.REG_X3,
		x86.REG_X4, x86.REG_X5, x86.REG_X6, x86.REG_X7,
	}
	gpIntRegisters = []int{
		x86.REG_AX, x86.REG_CX, x86.REG_DX, x86.REG_BX,
		x86.REG_SP, x86.REG_BP, x86.REG_SI, x86.REG_DI,
		x86.REG_R8, x86.REG_R9, x86.REG_R10, x86.REG_R11,
	}
	errFreeRegisterNotFound = errors.New("free register not found")
)

func isIntRegister(r int) bool {
	return gpIntRegisters[0] <= r && r <= gpIntRegisters[len(gpIntRegisters)-1]
}

func isFloatRegister(r int) bool {
	return gpFloatRegisters[0] <= r && r <= gpFloatRegisters[len(gpFloatRegisters)-1]
}

func newValueLocationStack() *valueLocationStack {
	return &valueLocationStack{
		usedRegisters: map[int]struct{}{},
	}
}

type valueLocationStack struct {
	stack         []*valueLocation
	sp            int
	usedRegisters map[int]struct{}
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

func (s *valueLocationStack) releaseRegister(reg int) {
	delete(s.usedRegisters, reg)
}

func (s *valueLocationStack) markRegisterUsed(reg int) {
	s.usedRegisters[reg] = struct{}{}
}

type generalPurposeRegisterType byte

const (
	gpTypeInt generalPurposeRegisterType = iota
	gpTypeFloat
)

// Search for unused registers, and if found, returns the resgister
// and mark it used.
func (s *valueLocationStack) takeFreeRegister(tp generalPurposeRegisterType) (int, error) {
	var targetRegs []int
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
		return candidate, nil
	}
	return 0, errFreeRegisterNotFound
}

// Search through the stack, and steal the register from the last used
// variable on the stack.
func (s *valueLocationStack) takeStealTargetFromUsedRegister(tp generalPurposeRegisterType) (target *valueLocation) {
	for i := 0; i < s.sp; i++ {
		loc := s.stack[i]
		if loc.onRegister() {
			switch tp {
			case gpTypeFloat:
				if isFloatRegister(*loc.register) {
					target = loc
					break
				}
			case gpTypeInt:
				if isIntRegister(*loc.register) {
					target = loc
					break
				}
			}
		}
	}
	return
}
