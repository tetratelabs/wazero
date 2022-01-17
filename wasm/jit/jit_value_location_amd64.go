//go:build amd64
// +build amd64

package jit

import (
	"fmt"
	"strings"

	"github.com/twitchyliquid64/golang-asm/obj/x86"
)

// Reserved registers.
const (
	// reservedRegisterForEngine R13: pointer to engine instance (i.e. *engine as uintptr)
	reservedRegisterForEngine = x86.REG_R13
	// reservedRegisterForStackBasePointer R14: stack base pointer (engine.stackBasePointer) in the current function call.
	reservedRegisterForStackBasePointer = x86.REG_R14
	// reservedRegisterForMemory R15: pointer to memory space (i.e. *[]byte as uintptr).
	reservedRegisterForMemory = x86.REG_R15
)

var (
	generalPurposeFloatRegisters = []int16{
		x86.REG_X0, x86.REG_X1, x86.REG_X2, x86.REG_X3,
		x86.REG_X4, x86.REG_X5, x86.REG_X6, x86.REG_X7,
		x86.REG_X8, x86.REG_X9, x86.REG_X10, x86.REG_X11,
		x86.REG_X12, x86.REG_X13, x86.REG_X14, x86.REG_X15,
	}
	// Note that we never invoke "call" instruction,
	// so we don't need to care about the calling convension.
	// TODO: Maybe it is safe just save rbp, rsp somewhere
	// in Go-allocated variables, and reuse these registers
	// in JITed functions and write them back before returns.
	unreservedGeneralPurposeIntRegisters = []int16{
		x86.REG_AX, x86.REG_CX, x86.REG_DX, x86.REG_BX,
		x86.REG_SI, x86.REG_DI, x86.REG_R8, x86.REG_R9,
		x86.REG_R10, x86.REG_R11, x86.REG_R12,
	}
)

func isIntRegister(r int16) bool {
	return unreservedGeneralPurposeIntRegisters[0] <= r && r <= unreservedGeneralPurposeIntRegisters[len(unreservedGeneralPurposeIntRegisters)-1]
}

func isFloatRegister(r int16) bool {
	return generalPurposeFloatRegisters[0] <= r && r <= generalPurposeFloatRegisters[len(generalPurposeFloatRegisters)-1]
}

type conditionalRegisterState byte

const (
	conditionalRegisterStateUnset conditionalRegisterState = iota
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

// valueLocation corresponds to each variable pushed onto the wazeroir (virtual) stack,
// and it has the information about where it exists in the physical machine.
// It might exist in registers, or maybe on in the non-virtual physical stack allocated in memory.
type valueLocation struct {
	regType generalPurposeRegisterType
	// Set to -1 if the value is stored in the memory stack.
	register int16
	// Set to conditionalRegisterStateUnset if the value is not on the conditional register.
	conditionalRegister conditionalRegisterState
	// This is the location of this value in the (virtual) stack,
	// even though if .register != -1, the value is not written into memory yet.
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
	return v.register != -1 && v.conditionalRegister == conditionalRegisterStateUnset
}

func (v *valueLocation) onStack() bool {
	return v.register == -1 && v.conditionalRegister == conditionalRegisterStateUnset
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
	s.markRegisterUsed(reg)
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
	loc.register = -1
	loc.conditionalRegister = conditionalRegisterStateUnset
}

func (s *valueLocationStack) markRegisterUnused(reg int16) {
	delete(s.usedRegisters, reg)
}

func (s *valueLocationStack) markRegisterUsed(reg int16) {
	s.usedRegisters[reg] = struct{}{}
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
