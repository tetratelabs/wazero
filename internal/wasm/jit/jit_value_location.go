package jit

import (
	"fmt"
	"strings"

	"github.com/tetratelabs/wazero/internal/wasm/jit/asm"
)

var (
	// unreservedGeneralPurposeIntRegisters contains unreserved general purpose registers of integer type.
	unreservedGeneralPurposeIntRegisters []asm.Register

	// unreservedGeneralPurposeFloatRegisters contains unreserved general purpose registers of scalar float type.
	unreservedGeneralPurposeFloatRegisters []asm.Register
)

func isNilRegister(r asm.Register) bool {
	return r == asm.NilRegister
}

func isIntRegister(r asm.Register) bool {
	return unreservedGeneralPurposeIntRegisters[0] <= r && r <= unreservedGeneralPurposeIntRegisters[len(unreservedGeneralPurposeIntRegisters)-1]
}

func isFloatRegister(r asm.Register) bool {
	return unreservedGeneralPurposeFloatRegisters[0] <= r && r <= unreservedGeneralPurposeFloatRegisters[len(unreservedGeneralPurposeFloatRegisters)-1]
}

// valueLocation corresponds to each variable pushed onto the wazeroir (virtual) stack,
// and it has the information about where it exists in the physical machine.
// It might exist in registers, or maybe on in the non-virtual physical stack allocated in memory.
type valueLocation struct {
	regType generalPurposeRegisterType
	// Set to asm.NilRegister if the value is stored in the memory stack.
	register asm.Register
	// Set to conditionalRegisterStateUnset if the value is not on the conditional register.
	conditionalRegister asm.ConditionalRegisterState
	// This is the location of this value in the memory stack at runtime,
	stackPointer uint64
}

func (v *valueLocation) registerType() (t generalPurposeRegisterType) {
	return v.regType
}

func (v *valueLocation) setRegisterType(t generalPurposeRegisterType) {
	v.regType = t
}

func (v *valueLocation) setRegister(reg asm.Register) {
	v.register = reg
	v.conditionalRegister = asm.ConditionalRegisterStateUnset
}

func (v *valueLocation) onRegister() bool {
	return v.register != asm.NilRegister && v.conditionalRegister == asm.ConditionalRegisterStateUnset
}

func (v *valueLocation) onStack() bool {
	return v.register == asm.NilRegister && v.conditionalRegister == asm.ConditionalRegisterStateUnset
}

func (v *valueLocation) onConditionalRegister() bool {
	return v.conditionalRegister != asm.ConditionalRegisterStateUnset
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
	return &valueLocationStack{usedRegisters: map[asm.Register]struct{}{}}
}

// valueLocationStack represents the wazeroir virtual stack
// where each item holds the location information about where it exists
// on the physical machine at runtime.
// Notably this is only used in the compilation phase, not runtime,
// and we change the state of this struct at every wazeroir operation we compile.
// In this way, we can see where the operands of a operation (for example,
// two variables for wazeroir add operation.) exist and check the necessity for
// moving the variable to registers to perform actual CPU instruction
// to achieve wazeroir's add operation.
type valueLocationStack struct {
	// stack holds all the variables.
	stack []*valueLocation
	// sp is the current stack pointer.
	sp uint64
	// usedRegisters stores the used registers.
	usedRegisters map[asm.Register]struct{}
	// stackPointerCeil tracks max(.sp) across the lifespan of this struct.
	stackPointerCeil uint64
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
	ret.usedRegisters = make(map[asm.Register]struct{}, len(ret.usedRegisters))
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
	ret.stackPointerCeil = s.stackPointerCeil
	return ret
}

// pushValueLocationOnRegister creates a new valueLocation with a given register and pushes onto
// the location stack.
func (s *valueLocationStack) pushValueLocationOnRegister(reg asm.Register) (loc *valueLocation) {
	loc = &valueLocation{register: reg, conditionalRegister: asm.ConditionalRegisterStateUnset}

	if isIntRegister(reg) {
		loc.setRegisterType(generalPurposeRegisterTypeInt)
	} else if isFloatRegister(reg) {
		loc.setRegisterType(generalPurposeRegisterTypeFloat)
	}
	s.push(loc)
	return
}

// pushValueLocationOnRegister creates a new valueLocation and pushes onto the location stack.
func (s *valueLocationStack) pushValueLocationOnStack() (loc *valueLocation) {
	loc = &valueLocation{register: asm.NilRegister, conditionalRegister: asm.ConditionalRegisterStateUnset}
	s.push(loc)
	return
}

// pushValueLocationOnRegister creates a new valueLocation with a given conditional register state
// and pushes onto the location stack.
func (s *valueLocationStack) pushValueLocationOnConditionalRegister(state asm.ConditionalRegisterState) (loc *valueLocation) {
	loc = &valueLocation{register: asm.NilRegister, conditionalRegister: state}
	s.push(loc)
	return
}

// push pushes to a given valueLocation onto the stack.
func (s *valueLocationStack) push(loc *valueLocation) {
	loc.stackPointer = s.sp
	if s.sp >= uint64(len(s.stack)) {
		// This case we need to grow the stack capacity by appending the item,
		// rather than indexing.
		s.stack = append(s.stack, loc)
	} else {
		s.stack[s.sp] = loc
	}
	if s.sp > s.stackPointerCeil {
		s.stackPointerCeil = s.sp
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
	loc.register = asm.NilRegister
	loc.conditionalRegister = asm.ConditionalRegisterStateUnset
}

func (s *valueLocationStack) markRegisterUnused(regs ...asm.Register) {
	for _, reg := range regs {
		delete(s.usedRegisters, reg)
	}
}

func (s *valueLocationStack) markRegisterUsed(regs ...asm.Register) {
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
func (s *valueLocationStack) takeFreeRegister(tp generalPurposeRegisterType) (reg asm.Register, found bool) {
	var targetRegs []asm.Register
	switch tp {
	case generalPurposeRegisterTypeFloat:
		targetRegs = unreservedGeneralPurposeFloatRegisters
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

func (s *valueLocationStack) takeFreeRegisters(tp generalPurposeRegisterType, num int) (regs []asm.Register, found bool) {
	var targetRegs []asm.Register
	switch tp {
	case generalPurposeRegisterTypeFloat:
		targetRegs = unreservedGeneralPurposeFloatRegisters
	case generalPurposeRegisterTypeInt:
		targetRegs = unreservedGeneralPurposeIntRegisters
	}

	regs = make([]asm.Register, 0, num)
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
