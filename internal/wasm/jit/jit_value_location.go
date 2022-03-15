package jit

import (
	"fmt"
	"strings"

	"github.com/tetratelabs/wazero/internal/wasm/buildoptions"
)

// nilRegister is used to indicate a register argument a variable is invalid and not an actual register.
const nilRegister int16 = -1

func isNilRegister(r int16) bool {
	return r == nilRegister
}

func isIntRegister(r int16) bool {
	return unreservedGeneralPurposeIntRegisters[0] <= r && r <= unreservedGeneralPurposeIntRegisters[len(unreservedGeneralPurposeIntRegisters)-1]
}

func isFloatRegister(r int16) bool {
	return generalPurposeFloatRegisters[0] <= r && r <= generalPurposeFloatRegisters[len(generalPurposeFloatRegisters)-1]
}

func isZeroRegister(r int16) bool {
	return r == zeroRegister
}

// conditionalRegisterState indicates a state of the conditional flag register.
// In arm64, conditional registers are defined as arm64.COND_*.
// In amd64, we define each flag value in value_locations_amd64.go
type conditionalRegisterState int16

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
	// This is the location of this value in the memory stack at runtime,
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
	usedRegisters map[int16]struct{}
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
	ret.stackPointerCeil = s.stackPointerCeil
	return ret
}

// pushValueLocationOnRegister creates a new valueLocation with a given register and pushes onto
// the location stack.
func (s *valueLocationStack) pushValueLocationOnRegister(reg int16) (loc *valueLocation) {
	loc = &valueLocation{register: reg, conditionalRegister: conditionalRegisterStateUnset}
	if buildoptions.IsDebugMode {
		if _, ok := s.usedRegisters[loc.register]; ok {
			panic("bug in compiler: try pushing a register which is already in use")
		}
	}

	if isIntRegister(reg) {
		loc.setRegisterType(generalPurposeRegisterTypeInt)
	} else if isFloatRegister(reg) {
		loc.setRegisterType(generalPurposeRegisterTypeFloat)
	}
	s.markRegisterUsed(reg)
	s.push(loc)
	return
}

// pushValueLocationOnRegister creates a new valueLocation and pushes onto the location stack.
func (s *valueLocationStack) pushValueLocationOnStack() (loc *valueLocation) {
	loc = &valueLocation{register: nilRegister, conditionalRegister: conditionalRegisterStateUnset}
	s.push(loc)
	return
}

// pushValueLocationOnRegister creates a new valueLocation with a given conditional register state
// and pushes onto the location stack.
func (s *valueLocationStack) pushValueLocationOnConditionalRegister(state conditionalRegisterState) (loc *valueLocation) {
	loc = &valueLocation{register: nilRegister, conditionalRegister: state}
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
		if !isZeroRegister(reg) {
			s.usedRegisters[reg] = struct{}{}
		}
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
