//go:build amd64
// +build amd64

package jit

import (
	"github.com/twitchyliquid64/golang-asm/obj/x86"

	"github.com/tetratelabs/wazero/wasm/wazeroir"
)

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
		x86.REG_SI, x86.REG_DI, x86.REG_R8, x86.REG_R9,
		x86.REG_R10, x86.REG_R11,
	}
)

func isIntRegister(r int16) bool {
	return gpIntRegisters[0] <= r && r <= gpIntRegisters[len(gpIntRegisters)-1]
}

func isFloatRegister(r int16) bool {
	return gpFloatRegisters[0] <= r && r <= gpFloatRegisters[len(gpFloatRegisters)-1]
}

type conditionalRegisterState byte

const (
	conditionalRegisterStateUnset conditionalRegisterState = 0 + iota
	conditionalRegisterStateE                              // ZF equal to zero
	conditionalRegisterStateNE                             //˜ZF not equal to zero
	conditionalRegisterStateS                              // SF negative
	conditionalRegisterStateNS                             // ˜SF non-negative
	conditionalRegisterStateG                              // ˜(SF xor OF) & ˜ ZF greater (signed >)
	conditionalRegisterStateGE                             // ˜(SF xor OF) greater or equal (signed >=)
	conditionalRegisterStateL                              // SF xor OF less (signed <)
	conditionalRegisterStateLE                             // (SF xor OF) | ZF less or equal (signed <=)
	conditionalRegisterStateA                              // ˜CF & ˜ZF above (unsigned >)
	conditionalRegisterStateAE                             // ˜CF above or equal (unsigned >=)
	conditionalRegisterStateB                              // CF below (unsigned <)
	conditionalRegisterStateBE                             // CF | ZF below or equal (unsigned <=)
)

type valueLocation struct {
	valueType wazeroir.SignLessType
	// Set to -1 if the value is stored in the memory stack.
	register int16
	// Set to conditionalRegisterStateUnset if the value is not on the conditional register.
	conditionalRegister conditionalRegisterState
	// This is the location of this value in the (virtual) stack,
	// even though if .register != -1, the value is not written into memory yet.
	stackPointer uint64
}

func (v *valueLocation) registerType() (t generalPurposeRegisterType) {
	switch v.valueType {
	case wazeroir.SignLessTypeI32, wazeroir.SignLessTypeI64:
		t = gpTypeInt
	case wazeroir.SignLessTypeF32, wazeroir.SignLessTypeF64:
		t = gpTypeFloat
	default:
		panic("unreachable")
	}
	return
}
func (v *valueLocation) setValueType(t wazeroir.SignLessType) {
	v.valueType = t
}

func (v *valueLocation) setRegister(reg int16) {
	v.register = reg
	v.conditionalRegister = conditionalRegisterStateUnset
}

func (v *valueLocation) onRegister() bool {
	return v.register != -1 && v.conditionalRegister == conditionalRegisterStateUnset
}

func (v *valueLocation) onStack() bool {
	return v.register == -1 && v.conditionalRegister == conditionalRegisterStateUnset
}

func (v *valueLocation) onConditionalRegister() bool {
	return v.conditionalRegister != conditionalRegisterStateUnset
}

func newValueLocationStack() *valueLocationStack {
	return &valueLocationStack{
		usedRegisters: map[int16]struct{}{},
	}
}

type valueLocationStack struct {
	stack         []*valueLocation
	sp            uint64
	usedRegisters map[int16]struct{}
}

func (s *valueLocationStack) pushValueOnRegister(reg int16) (loc *valueLocation) {
	loc = &valueLocation{register: reg, conditionalRegister: conditionalRegisterStateUnset}
	s.push(loc)
	return
}

func (s *valueLocationStack) pushValueOnStack() (loc *valueLocation) {
	loc = &valueLocation{register: -1, conditionalRegister: conditionalRegisterStateUnset}
	s.push(loc)
	return
}

func (s *valueLocationStack) pushValueOnConditionalRegister(state conditionalRegisterState) (loc *valueLocation) {
	loc = &valueLocation{register: -1, conditionalRegister: state}
	s.push(loc)
	return
}

func (s *valueLocationStack) push(loc *valueLocation) {
	loc.stackPointer = s.sp
	if s.sp >= uint64(len(s.stack)) {
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

func (s *valueLocationStack) releaseRegister(loc *valueLocation) {
	delete(s.usedRegisters, loc.register)
	loc.register = -1
}

func (s *valueLocationStack) markRegisterUsed(loc *valueLocation) {
	s.usedRegisters[loc.register] = struct{}{}
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
		return candidate, true
	}
	return 0, false
}

// Search through the stack, and steal the register from the last used
// variable on the stack.
func (s *valueLocationStack) takeStealTargetFromUsedRegister(tp generalPurposeRegisterType) (*valueLocation, bool) {
	for i := uint64(0); i < s.sp; i++ {
		loc := s.stack[i]
		if loc.onRegister() {
			switch tp {
			case gpTypeFloat:
				if isFloatRegister(loc.register) {
					return loc, true
				}
			case gpTypeInt:
				if isIntRegister(loc.register) {
					return loc, true
				}
			}
		}
	}
	return nil, false
}
