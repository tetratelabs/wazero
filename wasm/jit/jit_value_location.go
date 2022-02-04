//go:build amd64 || arm64
// +build amd64 arm64

package jit

import (
	"fmt"
	"strings"

	"github.com/tetratelabs/wazero/wasm/buildoptions"
)

// nilRegister is used to indicate a register argument a variable is invalid and not an actual register.
const nilRegister int16 = -1

func isNilRegister(r int16) bool {
	return r == nilRegister
}

type conditionalRegisterState byte

const conditionalRegisterStateUnset conditionalRegisterState = 0

// valueLocation corresponds to each variable pushed onto the wazeroir (virtual) stack,
// and it has the information about where it exists in the physical machine.
// It might exist in registers, or maybe on in the non-virtual physical stack allocated in memory.
type valueLocation struct {
	regType generalPurposeRegisterType
	// Set to nilRegister if the value is stored in the memory stack.
	register int16
	// Set to conditionalRegisterStateUnset if the value is not on the conditional register.
	conditionalRegister conditionalRegisterState
	// This is the location of this value in the (virtual) stack,
	// even though if .register != nilRegister, the value is not written into memory yet.
	stackPointer uint64
}

func (v *valueLocation) registerType() (t generalPurposeRegisterType) {
	return v.regType
}

func (v *valueLocation) setRegisterType(t generalPurposeRegisterType) {
	v.regType = t
}

func (v *valueLocation) setRegister(reg int16) {
	v.register = reg
	v.conditionalRegister = conditionalRegisterStateUnset
}

func (v *valueLocation) onRegister() bool {
	return v.register != nilRegister && v.conditionalRegister == conditionalRegisterStateUnset
}

func (v *valueLocation) onStack() bool {
	return v.register == nilRegister && v.conditionalRegister == conditionalRegisterStateUnset
}

func (v *valueLocation) onConditionalRegister() bool {
	return v.conditionalRegister != conditionalRegisterStateUnset
}

func (v *valueLocation) String() string {
	var location string
	if v.onStack() {
		location = fmt.Sprintf("stack(%d)", v.stackPointer)
	} else if v.onConditionalRegister() {
		location = fmt.Sprintf("conditional(%d)", v.conditionalRegister)
	} else if v.onRegister() {
		location = fmt.Sprintf("register(%d)", v.register)
	}
	return fmt.Sprintf("{type=%s,location=%s}", v.regType, location)
}

func newValueLocationStack() *valueLocationStack {
	return &valueLocationStack{
		usedRegisters: map[int16]struct{}{},
	}
}

// valueLocationStack represents the wazeroir virtual stack
// where each item holds the information about where it exists
// on the physical machine.
// Notably this is only used in the compilation phase, not runtime,
// and we change the state of this struct at every wazeroir operation we compile.
// In this way, we can see where the operands of a operation (for example,
// two variables for wazeroir add operation.) exist and check the neccesity for
// moving the variable to registers to perform actual CPU instruction
// to achieve wazeroir's add operation.
type valueLocationStack struct {
	// Holds all the variables.
	stack []*valueLocation
	// The current stack pointer.
	sp uint64
	// Stores the used registers.
	usedRegisters map[int16]struct{}
	// Records max(.sp) across the lifespan of this struct.
	maxStackPointer uint64
}

func (v *valueLocationStack) String() string {
	var stackStr []string
	for i := uint64(0); i < v.sp; i++ {
		stackStr = append(stackStr, v.stack[i].String())
	}
	var usedRegisters []string
	for reg := range v.usedRegisters {
		usedRegisters = append(usedRegisters, fmt.Sprintf("%d", reg))
	}
	return fmt.Sprintf("sp=%d, stack=[%s], used_registers=[%s]", v.sp, strings.Join(stackStr, ","), strings.Join(usedRegisters, ","))
}

func (s *valueLocationStack) clone() *valueLocationStack {
	ret := &valueLocationStack{}
	ret.sp = s.sp
	ret.usedRegisters = make(map[int16]struct{}, len(ret.usedRegisters))
	for r := range s.usedRegisters {
		ret.markRegisterUsed(r)
	}
	ret.stack = make([]*valueLocation, len(s.stack))
	for i, v := range s.stack {
		ret.stack[i] = &valueLocation{
			regType:             v.regType,
			conditionalRegister: v.conditionalRegister,
			stackPointer:        v.stackPointer,
			register:            v.register,
		}
	}
	ret.maxStackPointer = s.maxStackPointer
	return ret
}

func (s *valueLocationStack) pushValueOnRegister(reg int16) (loc *valueLocation) {
	loc = &valueLocation{register: reg, conditionalRegister: conditionalRegisterStateUnset}
	if buildoptions.IsDebugMode {
		if _, ok := s.usedRegisters[loc.register]; ok {
			panic("bug in compiler: try pushing a register which is already in use")
		}
	}
	s.markRegisterUsed(reg)
	s.push(loc)
	return
}

func (s *valueLocationStack) pushValueOnStack() (loc *valueLocation) {
	loc = &valueLocation{register: nilRegister, conditionalRegister: conditionalRegisterStateUnset}
	s.push(loc)
	return
}

func (s *valueLocationStack) pushValueOnConditionalRegister(state conditionalRegisterState) (loc *valueLocation) {
	loc = &valueLocation{register: nilRegister, conditionalRegister: state}
	s.push(loc)
	return
}

func (s *valueLocationStack) push(loc *valueLocation) {
	loc.stackPointer = s.sp
	if s.sp >= uint64(len(s.stack)) {
		// This case we need to grow the stack capacity by appending the item,
		// rather than indexing.
		s.stack = append(s.stack, loc)
	} else {
		s.stack[s.sp] = loc
	}
	if s.sp > s.maxStackPointer {
		s.maxStackPointer = s.sp
	}
	s.sp++
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
	s.markRegisterUnused(loc.register)
	loc.register = nilRegister
	loc.conditionalRegister = conditionalRegisterStateUnset
}

func (s *valueLocationStack) markRegisterUnused(regs ...int16) {
	for _, reg := range regs {
		delete(s.usedRegisters, reg)
	}
}

func (s *valueLocationStack) markRegisterUsed(regs ...int16) {
	for _, reg := range regs {
		s.usedRegisters[reg] = struct{}{}
	}
}

type generalPurposeRegisterType byte

const (
	generalPurposeRegisterTypeInt generalPurposeRegisterType = iota
	generalPurposeRegisterTypeFloat
)

func (tp generalPurposeRegisterType) String() (ret string) {
	switch tp {
	case generalPurposeRegisterTypeInt:
		ret = "int"
	case generalPurposeRegisterTypeFloat:
		ret = "float"
	}
	return
}

// takeFreeRegister searches for unused registers. Any found are marked used and returned.
func (s *valueLocationStack) takeFreeRegister(tp generalPurposeRegisterType) (reg int16, found bool) {
	var targetRegs []int16
	switch tp {
	case generalPurposeRegisterTypeFloat:
		targetRegs = generalPurposeFloatRegisters
	case generalPurposeRegisterTypeInt:
		targetRegs = unreservedGeneralPurposeIntRegisters
	}
	for _, candidate := range targetRegs {
		if _, ok := s.usedRegisters[candidate]; ok {
			continue
		}
		return candidate, true
	}
	return 0, false
}

func (s *valueLocationStack) takeFreeRegisters(tp generalPurposeRegisterType, num int) (regs []int16, found bool) {
	var targetRegs []int16
	switch tp {
	case generalPurposeRegisterTypeFloat:
		targetRegs = generalPurposeFloatRegisters
	case generalPurposeRegisterTypeInt:
		targetRegs = unreservedGeneralPurposeIntRegisters
	}

	regs = make([]int16, 0, num)
	for _, candidate := range targetRegs {
		if _, ok := s.usedRegisters[candidate]; ok {
			continue
		}
		regs = append(regs, candidate)
		if len(regs) == num {
			found = true
			break
		}
	}
	return
}

// Search through the stack, and steal the register from the last used
// variable on the stack.
func (s *valueLocationStack) takeStealTargetFromUsedRegister(tp generalPurposeRegisterType) (*valueLocation, bool) {
	for i := uint64(0); i < s.sp; i++ {
		loc := s.stack[i]
		if loc.onRegister() {
			switch tp {
			case generalPurposeRegisterTypeFloat:
				if isFloatRegister(loc.register) {
					return loc, true
				}
			case generalPurposeRegisterTypeInt:
				if isIntRegister(loc.register) {
					return loc, true
				}
			}
		}
	}
	return nil, false
}

// findValueForRegister returns the valueLocation of the given register or nil if not found.
// If not found, return nil.
func (s *valueLocationStack) findValueForRegister(reg int16) *valueLocation {
	for i := uint64(0); i < s.sp; i++ {
		loc := s.stack[i]
		if loc.register == reg {
			return loc
		}
	}
	return nil
}
