package compiler

import (
	"fmt"
	"strings"

	"github.com/tetratelabs/wazero/internal/asm"
)

var (
	// unreservedGeneralPurposeRegisters contains unreserved general purpose registers.
	unreservedGeneralPurposeRegisters []asm.Register

	// unreservedVectorRegisters contains unreserved vector registers.
	unreservedVectorRegisters []asm.Register
)

func isNilRegister(r asm.Register) bool {
	return r == asm.NilRegister
}

func isIntRegister(r asm.Register) bool {
	return unreservedGeneralPurposeRegisters[0] <= r && r <= unreservedGeneralPurposeRegisters[len(unreservedGeneralPurposeRegisters)-1]
}

func isVectorRegister(r asm.Register) bool {
	return unreservedVectorRegisters[0] <= r && r <= unreservedVectorRegisters[len(unreservedVectorRegisters)-1]
}

// valueLocation corresponds to each variable pushed onto the wazeroir (virtual) stack,
// and it has the information about where it exists in the physical machine.
// It might exist in registers, or maybe on in the non-virtual physical stack allocated in memory.
type valueLocation struct {
	regType registerType
	// register is set to asm.NilRegister if the value is stored in the memory stack.
	register asm.Register
	// conditionalRegister is set to conditionalRegisterStateUnset if the value is not on the conditional register.
	conditionalRegister asm.ConditionalRegisterState
	// stackPointer is the location of this value in the memory stack at runtime.
	stackPointer uint64
}

func (v *valueLocation) registerType() (t registerType) {
	return v.regType
}

func (v *valueLocation) setRegisterType(t registerType) {
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

func (v *valueLocationStack) clone() *valueLocationStack {
	ret := &valueLocationStack{}
	ret.sp = v.sp
	ret.usedRegisters = make(map[asm.Register]struct{}, len(ret.usedRegisters))
	for r := range v.usedRegisters {
		ret.markRegisterUsed(r)
	}
	ret.stack = make([]*valueLocation, len(v.stack))
	for i, v := range v.stack {
		ret.stack[i] = &valueLocation{
			regType:             v.regType,
			conditionalRegister: v.conditionalRegister,
			stackPointer:        v.stackPointer,
			register:            v.register,
		}
	}
	ret.stackPointerCeil = v.stackPointerCeil
	return ret
}

// pushValueLocationOnRegister creates a new valueLocation with a given register and pushes onto
// the location stack.
func (v *valueLocationStack) pushValueLocationOnRegister(reg asm.Register) (loc *valueLocation) {
	loc = &valueLocation{register: reg, conditionalRegister: asm.ConditionalRegisterStateUnset}

	if isIntRegister(reg) {
		loc.setRegisterType(registerTypeGeneralPurpose)
	} else if isVectorRegister(reg) {
		loc.setRegisterType(registerTypeVector)
	}
	v.push(loc)
	return
}

// pushValueLocationOnRegister creates a new valueLocation and pushes onto the location stack.
func (v *valueLocationStack) pushValueLocationOnStack() (loc *valueLocation) {
	loc = &valueLocation{register: asm.NilRegister, conditionalRegister: asm.ConditionalRegisterStateUnset}
	v.push(loc)
	return
}

// pushValueLocationOnRegister creates a new valueLocation with a given conditional register state
// and pushes onto the location stack.
func (v *valueLocationStack) pushValueLocationOnConditionalRegister(state asm.ConditionalRegisterState) (loc *valueLocation) {
	loc = &valueLocation{register: asm.NilRegister, conditionalRegister: state}
	v.push(loc)
	return
}

// push pushes to a given valueLocation onto the stack.
func (v *valueLocationStack) push(loc *valueLocation) {
	loc.stackPointer = v.sp
	if v.sp >= uint64(len(v.stack)) {
		// This case we need to grow the stack capacity by appending the item,
		// rather than indexing.
		v.stack = append(v.stack, loc)
	} else {
		v.stack[v.sp] = loc
	}
	if v.sp > v.stackPointerCeil {
		v.stackPointerCeil = v.sp
	}
	v.sp++
}

func (v *valueLocationStack) pop() (loc *valueLocation) {
	v.sp--
	loc = v.stack[v.sp]
	return
}

func (v *valueLocationStack) peek() (loc *valueLocation) {
	loc = v.stack[v.sp-1]
	return
}

func (v *valueLocationStack) releaseRegister(loc *valueLocation) {
	v.markRegisterUnused(loc.register)
	loc.register = asm.NilRegister
	loc.conditionalRegister = asm.ConditionalRegisterStateUnset
}

func (v *valueLocationStack) markRegisterUnused(regs ...asm.Register) {
	for _, reg := range regs {
		delete(v.usedRegisters, reg)
	}
}

func (v *valueLocationStack) markRegisterUsed(regs ...asm.Register) {
	for _, reg := range regs {
		v.usedRegisters[reg] = struct{}{}
	}
}

type registerType byte

const (
	registerTypeGeneralPurpose registerType = iota
	// registerTypeVector represents a vector register which can be used for either scalar float
	// operation or SIMD vector operation depending on the instrcution by which the register is used.
	//
	// Note: In normal assembly language, scalar float and vector register have different notations as
	// Vn is for vectors and Qn is for scalara floats on arm64 for example. But on phyisical hardware,
	// they are placed on the same locations. (Qn means the lower 64-bit of Vn vector register on arm64).
	//
	// In wazero, for the sake of simplicity in the register allocation, we intentionally ambiguate the two types
	// and delegate the decision to the assembler which is aware of the instruction types for which these registers are used.
	registerTypeVector
)

func (tp registerType) String() (ret string) {
	switch tp {
	case registerTypeGeneralPurpose:
		ret = "gp"
	case registerTypeVector:
		ret = "vector"
	}
	return
}

// takeFreeRegister searches for unused registers. Any found are marked used and returned.
func (v *valueLocationStack) takeFreeRegister(tp registerType) (reg asm.Register, found bool) {
	var targetRegs []asm.Register
	switch tp {
	case registerTypeVector:
		targetRegs = unreservedVectorRegisters
	case registerTypeGeneralPurpose:
		targetRegs = unreservedGeneralPurposeRegisters
	}
	for _, candidate := range targetRegs {
		if _, ok := v.usedRegisters[candidate]; ok {
			continue
		}
		return candidate, true
	}
	return 0, false
}

func (v *valueLocationStack) takeFreeRegisters(tp registerType, num int) (regs []asm.Register, found bool) {
	var targetRegs []asm.Register
	switch tp {
	case registerTypeVector:
		targetRegs = unreservedVectorRegisters
	case registerTypeGeneralPurpose:
		targetRegs = unreservedGeneralPurposeRegisters
	}

	regs = make([]asm.Register, 0, num)
	for _, candidate := range targetRegs {
		if _, ok := v.usedRegisters[candidate]; ok {
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
func (v *valueLocationStack) takeStealTargetFromUsedRegister(tp registerType) (*valueLocation, bool) {
	for i := uint64(0); i < v.sp; i++ {
		loc := v.stack[i]
		if loc.onRegister() {
			switch tp {
			case registerTypeVector:
				if isVectorRegister(loc.register) {
					return loc, true
				}
			case registerTypeGeneralPurpose:
				if isIntRegister(loc.register) {
					return loc, true
				}
			}
		}
	}
	return nil, false
}
