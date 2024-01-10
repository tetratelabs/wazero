package amd64

import (
	"fmt"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
)

type operand struct {
	kind  operandKind
	r     regalloc.VReg
	imm32 uint32
	amode amode
}

type operandKind byte

const (
	// operandKindReg is an operand which is an integer Register.
	operandKindReg operandKind = iota + 1

	// operandKindMem is an operand which is either an integer Register or a value in Memory.  This can denote an 8, 16,
	// 32, 64, or 128 bit value.
	operandKindMem

	// operandKindRegMemImm is either an integer Register, a value in Memory or an Immediate.
	operandImm32
)

func (o *operand) format(_64 bool) string {
	switch o.kind {
	case operandKindReg:
		return formatVRegSized(o.r, _64)
	case operandKindMem:
		return o.amode.String()
	case operandImm32:
		return fmt.Sprintf("$%d", int32(o.imm32))
	default:
		panic("BUG: invalid operand kind")
	}
}

func newOperandReg(r regalloc.VReg) operand {
	return operand{kind: operandKindReg, r: r}
}

func newOperandImm32(imm32 uint32) operand {
	return operand{kind: operandImm32, imm32: imm32}
}

// amode is a memory operand (addressing mode).
type amode struct {
	kind  amodeKind
	imm32 uint32
	base  regalloc.VReg

	// For amodeRegRegShit:
	index regalloc.VReg
	shift byte // 0, 1, 2, 3

	// For amodeRipRelative.
	// If kind == amodeRipRelative, and label is invalid,
	// then imm32 should represent the resolved address.
	label backend.Label
}

type amodeKind byte

const (
	// amodeRegRegShit calcualtes sign-extend-32-to-64(Immediate) + base
	amodeImmReg amodeKind = iota + 1

	// amodeRegRegShit calculates sign-extend-32-to-64(Immediate) + base + (Register2 << Shift)
	amodeRegRegShit

	// amodeRipRelative is a memory operand with RIP-relative addressing mode.
	amodeRipRelative

	// TODO: there are other addressing modes such as the one with base register is absent.
)

// String implements fmt.Stringer.
func (a *amode) String() string {
	switch a.kind {
	case amodeImmReg:
		return fmt.Sprintf("%d(%s)", int32(a.imm32), formatVRegSized(a.base, true))
	case amodeRegRegShit:
		return fmt.Sprintf(
			"%d(%s,%s,%d)",
			int32(a.imm32), formatVRegSized(a.base, true), formatVRegSized(a.index, true), 1<<a.shift)
	case amodeRipRelative:
		if a.label != backend.LabelInvalid {
			return fmt.Sprintf("%s(%%rip)", a.label)
		} else {
			return fmt.Sprintf("%d(%%rip)", int32(a.imm32))
		}
	}
	panic("BUG: invalid amode kind")
}
